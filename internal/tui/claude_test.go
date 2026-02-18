package tui

import (
	"encoding/json"
	"testing"
)

func TestFindContentArray_MessageContent(t *testing.T) {
	// Standard assistant event: content is nested in message.content
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Hello world"}]},"session_id":"sess_123"}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	contentRaw := findContentArray(raw)
	if contentRaw == nil {
		t.Fatal("findContentArray returned nil for assistant event with message.content")
	}

	var blocks []json.RawMessage
	if err := json.Unmarshal(contentRaw, &blocks); err != nil {
		t.Fatal(err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(blocks))
	}
}

func TestFindContentArray_DirectContent(t *testing.T) {
	// Fallback: content is at top level
	input := `{"type":"assistant","content":[{"type":"text","text":"Hello"}]}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	contentRaw := findContentArray(raw)
	if contentRaw == nil {
		t.Fatal("findContentArray returned nil for direct content")
	}
}

func TestExtractFromAssistant(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Let me check docker."},{"type":"tool_use","id":"tu_xxx","name":"docker_ps","input":{"all":false}}]},"session_id":"sess_123"}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	events := extractFromAssistant(raw)
	if len(events) != 2 {
		t.Fatalf("expected 2 events (text + tool_use), got %d", len(events))
	}

	if events[0].Type != "assistant_chunk" || events[0].Content != "Let me check docker." {
		t.Errorf("event[0]: expected assistant_chunk with text, got %+v", events[0])
	}
	if events[1].Type != "tool_use" || events[1].Tool != "docker_ps" {
		t.Errorf("event[1]: expected tool_use docker_ps, got %+v", events[1])
	}
}

func TestExtractToolUseFromAssistant(t *testing.T) {
	input := `{"type":"assistant","message":{"content":[{"type":"text","text":"Some text"},{"type":"tool_use","id":"tu_xxx","name":"docker_stats","input":{}}]}}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	events := extractToolUseFromAssistant(raw)
	if len(events) != 1 {
		t.Fatalf("expected 1 event (tool_use only), got %d", len(events))
	}
	if events[0].Type != "tool_use" || events[0].Tool != "docker_stats" {
		t.Errorf("expected tool_use docker_stats, got %+v", events[0])
	}
}

func TestProcessStreamEvent_TextDelta(t *testing.T) {
	input := `{"type":"stream_event","event":{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	ch := make(chan StreamEvent, 10)
	processStreamEvent(raw, ch)

	select {
	case event := <-ch:
		if event.Type != "assistant_chunk" || event.Content != "Hello" {
			t.Errorf("expected assistant_chunk 'Hello', got %+v", event)
		}
	default:
		t.Fatal("no event received from processStreamEvent for text_delta")
	}
}

func TestProcessStreamEvent_ToolUseStart(t *testing.T) {
	input := `{"type":"stream_event","event":{"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tu_xxx","name":"docker_ps","input":{}}}}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	ch := make(chan StreamEvent, 10)
	processStreamEvent(raw, ch)

	select {
	case event := <-ch:
		if event.Type != "tool_use" || event.Tool != "docker_ps" {
			t.Errorf("expected tool_use docker_ps, got %+v", event)
		}
	default:
		t.Fatal("no event received from processStreamEvent for tool_use start")
	}
}

func TestProcessStreamEvent_InputJsonDelta(t *testing.T) {
	// input_json_delta should be silently ignored (not text)
	input := `{"type":"stream_event","event":{"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"all\":"}}}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	ch := make(chan StreamEvent, 10)
	processStreamEvent(raw, ch)

	select {
	case event := <-ch:
		t.Errorf("expected no event for input_json_delta, got %+v", event)
	default:
		// Correct: no event emitted
	}
}

func TestExtractSessionID(t *testing.T) {
	input := `{"type":"result","result":"Hello","session_id":"sess_abc123"}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	sid := extractSessionID(raw)
	if sid != "sess_abc123" {
		t.Errorf("expected session_id 'sess_abc123', got '%s'", sid)
	}
}

func TestExtractSessionID_Missing(t *testing.T) {
	input := `{"type":"result","result":"Hello"}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	sid := extractSessionID(raw)
	if sid != "" {
		t.Errorf("expected empty session_id, got '%s'", sid)
	}
}

func TestFindContentArray_NoContent(t *testing.T) {
	input := `{"type":"system","subtype":"init"}`
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}

	contentRaw := findContentArray(raw)
	if contentRaw != nil {
		t.Error("expected nil for event with no content")
	}
}
