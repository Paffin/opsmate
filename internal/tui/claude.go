package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// StreamEvent represents a single streamed event from the Claude process.
type StreamEvent struct {
	Type      string
	Content   string
	Tool      string
	Input     string
	SessionID string
}

// debugLog writes to OPSMATE_DEBUG log file when set.
func debugLog(format string, args ...interface{}) {
	path := os.Getenv("OPSMATE_DEBUG")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck
	logger := log.New(f, "", log.LstdFlags)
	logger.Printf(format, args...)
}

// RunQuery runs a Claude query and streams events back on a channel.
// sessionID can be empty for a new session, or a prior session ID to resume.
func RunQuery(ctx context.Context, prompt, sessionID, mcpConfigPath, workDir string) (<-chan StreamEvent, error) {
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found in PATH")
	}

	// Normalize path separators for cross-platform compatibility
	mcpConfigPath = filepath.ToSlash(mcpConfigPath)

	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--mcp-config", mcpConfigPath,
	}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}

	debugLog("Running: %s %s", claudeBin, strings.Join(args, " "))
	debugLog("Prompt (%d bytes)", len(prompt))

	cmd := exec.CommandContext(ctx, claudeBin, args...)
	cmd.Dir = workDir
	// Inherit parent environment (PATH, HOME, API keys, etc.)
	cmd.Env = os.Environ()

	// Pipe stdin — deliver prompt via stdin for reliable Unicode handling
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	// Write prompt to stdin and close to signal EOF
	_, writeErr := io.WriteString(stdinPipe, prompt)
	if writeErr != nil {
		debugLog("stdin write error: %v", writeErr)
		_ = stdinPipe.Close()
		return nil, fmt.Errorf("write prompt to stdin: %w", writeErr)
	}
	_ = stdinPipe.Close()

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)

		// Read stderr in a separate goroutine
		var stderrLines []string
		var stderrMu sync.Mutex
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			sc := bufio.NewScanner(stderr)
			for sc.Scan() {
				line := sc.Text()
				debugLog("STDERR: %s", line)
				stderrMu.Lock()
				stderrLines = append(stderrLines, line)
				stderrMu.Unlock()
			}
		}()

		scanner := bufio.NewScanner(stdout)
		// Increase scanner buffer for large JSON lines
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, len(buf))

		lineCount := 0
		eventCount := 0

		for scanner.Scan() {
			line := scanner.Text()
			lineCount++
			debugLog("LINE %d: %s", lineCount, line)

			if line == "" {
				continue
			}

			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				debugLog("JSON parse error: %v (line: %s)", err, line)
				// Maybe it's plain text output (non-JSON)
				ch <- StreamEvent{Type: "assistant_chunk", Content: line + "\n"}
				eventCount++
				continue
			}

			// Get type field
			typeRaw, hasType := raw["type"]
			if !hasType {
				debugLog("No 'type' field in: %s", line)
				// Try to extract any text content from the raw JSON
				if resultRaw, ok := raw["result"]; ok {
					var resultStr string
					if err := json.Unmarshal(resultRaw, &resultStr); err == nil && resultStr != "" {
						ch <- StreamEvent{Type: "assistant_chunk", Content: resultStr}
						eventCount++
					}
				}
				continue
			}
			var eventType string
			if err := json.Unmarshal(typeRaw, &eventType); err != nil {
				debugLog("Type unmarshal error: %v", err)
				continue
			}

			debugLog("Event type: %s", eventType)

			switch eventType {
			case "content_block_delta":
				processContentBlockDelta(raw, ch)
				eventCount++

			case "content_block_start":
				processContentBlockStart(raw, ch)
				eventCount++

			case "message_start", "message_delta", "message_stop",
				"content_block_stop", "ping":
				// Lifecycle events — ignore
				eventCount++

			case "stream_event":
				processStreamEvent(raw, ch)
				eventCount++

			case "assistant":
				events := extractFromAssistant(raw)
				for _, e := range events {
					ch <- e
				}
				eventCount++

			case "result":
				sid := extractSessionID(raw)
				if eventCount <= 1 {
					if resultRaw, ok := raw["result"]; ok {
						var resultStr string
						if err := json.Unmarshal(resultRaw, &resultStr); err == nil && resultStr != "" {
							ch <- StreamEvent{Type: "assistant_chunk", Content: resultStr}
						}
					}
				}
				ch <- StreamEvent{Type: "message_end", SessionID: sid}
				eventCount = 0

			default:
				debugLog("Unknown event type: %s", eventType)
			}
		}

		if err := scanner.Err(); err != nil {
			debugLog("Scanner error: %v", err)
			select {
			case <-ctx.Done():
			default:
				ch <- StreamEvent{Type: "error", Content: fmt.Sprintf("read error: %v", err)}
			}
		}

		// Wait for stderr goroutine to finish
		wg.Wait()

		// Wait for process to finish
		waitErr := cmd.Wait()

		debugLog("Process exited. Lines read: %d, exit error: %v", lineCount, waitErr)

		// If process exited with error or we got no output, show stderr
		stderrMu.Lock()
		stderrStr := strings.TrimSpace(strings.Join(stderrLines, "\n"))
		stderrMu.Unlock()

		if waitErr != nil {
			select {
			case <-ctx.Done():
			default:
				errMsg := fmt.Sprintf("claude exited: %v", waitErr)
				if stderrStr != "" {
					errMsg = stderrStr
				}
				ch <- StreamEvent{Type: "error", Content: errMsg}
			}
			return
		}

		if lineCount == 0 {
			debugLog("No output. stderr: %s", stderrStr)
			if stderrStr != "" {
				ch <- StreamEvent{Type: "error", Content: stderrStr}
			} else {
				ch <- StreamEvent{Type: "error", Content: "No response from Claude CLI"}
			}
		}
	}()

	return ch, nil
}

// processContentBlockDelta handles direct content_block_delta events.
func processContentBlockDelta(raw map[string]json.RawMessage, ch chan<- StreamEvent) {
	deltaRaw, ok := raw["delta"]
	if !ok {
		return
	}
	var delta map[string]json.RawMessage
	if err := json.Unmarshal(deltaRaw, &delta); err != nil {
		return
	}
	dtRaw, ok := delta["type"]
	if !ok {
		return
	}
	var dt string
	if err := json.Unmarshal(dtRaw, &dt); err != nil {
		return
	}
	if dt == "text_delta" {
		if textRaw, ok := delta["text"]; ok {
			var text string
			if err := json.Unmarshal(textRaw, &text); err == nil {
				ch <- StreamEvent{Type: "assistant_chunk", Content: text}
			}
		}
	}
}

// processContentBlockStart handles direct content_block_start events.
func processContentBlockStart(raw map[string]json.RawMessage, ch chan<- StreamEvent) {
	cbRaw, ok := raw["content_block"]
	if !ok {
		return
	}
	var cb map[string]json.RawMessage
	if err := json.Unmarshal(cbRaw, &cb); err != nil {
		return
	}
	cbTypeRaw, ok := cb["type"]
	if !ok {
		return
	}
	var cbType string
	if err := json.Unmarshal(cbTypeRaw, &cbType); err != nil {
		return
	}
	if cbType == "tool_use" {
		if nameRaw, ok := cb["name"]; ok {
			var name string
			if err := json.Unmarshal(nameRaw, &name); err == nil {
				ch <- StreamEvent{Type: "tool_use", Tool: name}
			}
		}
	}
}

// processStreamEvent handles {"type":"stream_event","event":{...}} messages.
func processStreamEvent(raw map[string]json.RawMessage, ch chan<- StreamEvent) {
	eventRaw, ok := raw["event"]
	if !ok {
		return
	}
	var event map[string]json.RawMessage
	if err := json.Unmarshal(eventRaw, &event); err != nil {
		return
	}

	subTypeRaw, ok := event["type"]
	if !ok {
		return
	}
	var subType string
	if err := json.Unmarshal(subTypeRaw, &subType); err != nil {
		return
	}

	switch subType {
	case "content_block_start":
		processContentBlockStart(event, ch)
	case "content_block_delta":
		processContentBlockDelta(event, ch)
	}
}

// extractSessionID tries to get session_id from a raw JSON map.
func extractSessionID(raw map[string]json.RawMessage) string {
	if sidRaw, ok := raw["session_id"]; ok {
		var sid string
		if err := json.Unmarshal(sidRaw, &sid); err == nil {
			return sid
		}
	}
	return ""
}

// extractFromAssistant parses text and tool_use blocks from an "assistant" event.
func extractFromAssistant(raw map[string]json.RawMessage) []StreamEvent {
	var events []StreamEvent

	contentRaw := findContentArray(raw)
	if contentRaw == nil {
		return events
	}

	var blocks []json.RawMessage
	if err := json.Unmarshal(contentRaw, &blocks); err != nil {
		return events
	}

	for _, blockRaw := range blocks {
		var block map[string]json.RawMessage
		if err := json.Unmarshal(blockRaw, &block); err != nil {
			continue
		}

		blockTypeRaw, ok := block["type"]
		if !ok {
			continue
		}
		var blockType string
		if err := json.Unmarshal(blockTypeRaw, &blockType); err != nil {
			continue
		}

		switch blockType {
		case "text":
			if textRaw, ok := block["text"]; ok {
				var text string
				if err := json.Unmarshal(textRaw, &text); err == nil {
					events = append(events, StreamEvent{Type: "assistant_chunk", Content: text})
				}
			}
		case "tool_use":
			toolName := ""
			if nameRaw, ok := block["name"]; ok {
				json.Unmarshal(nameRaw, &toolName) //nolint:errcheck
			}
			events = append(events, StreamEvent{Type: "tool_use", Tool: toolName})
		}
	}

	return events
}

// findContentArray locates the content array in a raw JSON message.
func findContentArray(raw map[string]json.RawMessage) json.RawMessage {
	if msgRaw, ok := raw["message"]; ok {
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(msgRaw, &msg); err == nil {
			if contentRaw, ok := msg["content"]; ok {
				return contentRaw
			}
		}
	}
	if contentRaw, ok := raw["content"]; ok {
		return contentRaw
	}
	return nil
}
