package chatui

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
		defer cmd.Wait()

		scanner := bufio.NewScanner(stdout)
		// Increase scanner buffer for large JSON lines
		buf := make([]byte, 1024*1024)
		scanner.Buffer(buf, len(buf))

		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}

			var raw map[string]json.RawMessage
			if err := json.Unmarshal([]byte(line), &raw); err != nil {
				continue
			}

			// Check for final result line: has a "result" string field
			if resultRaw, ok := raw["result"]; ok {
				var resultStr string
				if err := json.Unmarshal(resultRaw, &resultStr); err == nil && resultStr != "" {
					// Extract session_id if present
					sid := extractSessionID(raw)
					if resultStr != "" {
						ch <- StreamEvent{Type: "assistant_chunk", Content: resultStr}
					}
					ch <- StreamEvent{Type: "message_end", SessionID: sid}
					continue
				}
			}

			// Check for stream events with type field
			typeRaw, hasType := raw["type"]
			if !hasType {
				continue
			}
			var eventType string
			if err := json.Unmarshal(typeRaw, &eventType); err != nil {
				continue
			}

			switch eventType {
			case "assistant":
				// Has content array
				events := extractTextFromContent(raw)
				for _, e := range events {
					ch <- e
				}

			case "result":
				// Final result event
				sid := extractSessionID(raw)
				// Try to get result text
				if resultRaw, ok := raw["result"]; ok {
					var resultStr string
					if err := json.Unmarshal(resultRaw, &resultStr); err == nil && resultStr != "" {
						ch <- StreamEvent{Type: "assistant_chunk", Content: resultStr}
					}
				}
				ch <- StreamEvent{Type: "message_end", SessionID: sid}

			default:
				// Try to find text_delta in nested event structure
				if eventRaw, ok := raw["event"]; ok {
					var nested map[string]json.RawMessage
					if err := json.Unmarshal(eventRaw, &nested); err == nil {
						if deltaRaw, ok := nested["delta"]; ok {
							var delta map[string]json.RawMessage
							if err := json.Unmarshal(deltaRaw, &delta); err == nil {
								if deltaTypeRaw, ok := delta["type"]; ok {
									var deltaType string
									if err := json.Unmarshal(deltaTypeRaw, &deltaType); err == nil && deltaType == "text_delta" {
										if textRaw, ok := delta["text"]; ok {
											var text string
											if err := json.Unmarshal(textRaw, &text); err == nil {
												ch <- StreamEvent{Type: "assistant_chunk", Content: text}
											}
										}
									}
								}
							}
						}
					}
				}
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

// extractTextFromContent parses content blocks from an "assistant" event.
func extractTextFromContent(raw map[string]json.RawMessage) []StreamEvent {
	var events []StreamEvent

	contentRaw, ok := raw["content"]
	if !ok {
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
			toolInput := ""
			if nameRaw, ok := block["name"]; ok {
				json.Unmarshal(nameRaw, &toolName)
			}
			if inputRaw, ok := block["input"]; ok {
				// Serialize input back to string for display
				toolInput = string(inputRaw)
			}
			events = append(events, StreamEvent{Type: "tool_use", Tool: toolName, Input: toolInput})
		}
	}

	return events
}
