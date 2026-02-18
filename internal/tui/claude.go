package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// StreamEvent represents a single streamed event from the Claude process.
type StreamEvent struct {
	Type      string
	Content   string
	Tool      string
	Input     string
	SessionID string
}

// RunQuery runs a Claude query and streams events back on a channel.
// sessionID can be empty for a new session, or a prior session ID to resume.
func RunQuery(ctx context.Context, prompt, sessionID, mcpConfigPath, workDir string) (<-chan StreamEvent, error) {
	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return nil, fmt.Errorf("claude CLI not found in PATH")
	}

	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--include-partial-messages",
		"--mcp-config", mcpConfigPath,
	}
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, claudeBin, args...)
	cmd.Dir = workDir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start claude process: %w", err)
	}

	ch := make(chan StreamEvent, 64)

	go func() {
		defer close(ch)
		defer cmd.Wait() //nolint:errcheck

		scanner := bufio.NewScanner(stdout)
		// Increase scanner buffer for large JSON lines
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, len(buf))

		// Track content sources to avoid duplication.
		// hasStreamedContent: true when stream_event deltas were received (real-time).
		// hasAnyContent: true when any text was shown (from streaming or assistant events).
		hasStreamedContent := false
		hasAnyContent := false

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				continue
			}

			// Get type field
			typeRaw, hasType := raw["type"]
			if !hasType {
				continue
			}
			var eventType string
			if err := json.Unmarshal(typeRaw, &eventType); err != nil {
				continue
			}

			switch eventType {
			case "stream_event":
				// Real-time streaming deltas from --include-partial-messages
				processStreamEvent(raw, ch)
				hasStreamedContent = true
				hasAnyContent = true

			case "assistant":
				// Complete assistant message (verbose turn output).
				// Only extract text if we haven't received streaming deltas,
				// otherwise we'd duplicate the text.
				if !hasStreamedContent {
					events := extractFromAssistant(raw)
					for _, e := range events {
						ch <- e
					}
					hasAnyContent = true
				} else {
					// Still extract tool_use events in case streaming missed them
					events := extractToolUseFromAssistant(raw)
					for _, e := range events {
						ch <- e
					}
				}
				// Reset streaming flag for the next turn
				hasStreamedContent = false

			case "result":
				sid := extractSessionID(raw)
				if !hasStreamedContent && !hasAnyContent {
					// Last resort: no content from streaming or assistant events.
					// Show the result text directly.
					if resultRaw, ok := raw["result"]; ok {
						var resultStr string
						if err := json.Unmarshal(resultRaw, &resultStr); err == nil && resultStr != "" {
							ch <- StreamEvent{Type: "assistant_chunk", Content: resultStr}
						}
					}
				}
				ch <- StreamEvent{Type: "message_end", SessionID: sid}
				hasStreamedContent = false
				hasAnyContent = false
			}
		}

		if err := scanner.Err(); err != nil {
			select {
			case <-ctx.Done():
			default:
				ch <- StreamEvent{Type: "error", Content: err.Error()}
			}
		}
	}()

	return ch, nil
}

// processStreamEvent handles {"type":"stream_event","event":{...}} messages.
// These contain real-time content_block_delta and content_block_start events.
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
		// Detect tool_use block starts to show [tool_name]
		cbRaw, ok := event["content_block"]
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

	case "content_block_delta":
		// Text deltas for real-time streaming
		deltaRaw, ok := event["delta"]
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
// Assistant events wrap content inside message.content:
//
//	{"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}
func extractFromAssistant(raw map[string]json.RawMessage) []StreamEvent {
	return extractContentBlocks(raw, true)
}

// extractToolUseFromAssistant extracts only tool_use events from an assistant message.
// Used when streaming deltas already provided the text content.
func extractToolUseFromAssistant(raw map[string]json.RawMessage) []StreamEvent {
	return extractContentBlocks(raw, false)
}

// extractContentBlocks parses content blocks from an assistant event.
// If includeText is false, only tool_use blocks are returned.
func extractContentBlocks(raw map[string]json.RawMessage, includeText bool) []StreamEvent {
	var events []StreamEvent

	// Assistant events nest content inside "message"
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
			if includeText {
				if textRaw, ok := block["text"]; ok {
					var text string
					if err := json.Unmarshal(textRaw, &text); err == nil {
						events = append(events, StreamEvent{Type: "assistant_chunk", Content: text})
					}
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
// It checks both direct "content" field and nested "message.content".
func findContentArray(raw map[string]json.RawMessage) json.RawMessage {
	// Try message.content first (standard assistant event format)
	if msgRaw, ok := raw["message"]; ok {
		var msg map[string]json.RawMessage
		if err := json.Unmarshal(msgRaw, &msg); err == nil {
			if contentRaw, ok := msg["content"]; ok {
				return contentRaw
			}
		}
	}

	// Fallback: try direct content field
	if contentRaw, ok := raw["content"]; ok {
		return contentRaw
	}

	return nil
}
