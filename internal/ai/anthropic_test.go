package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseAnthropicStream_TextOnly(t *testing.T) {
	sse := strings.Join([]string{
		`event: message_start`,
		`data: {"type":"message_start","message":{"id":"msg_1"}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":", world"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	var streamed []string
	result, err := parseAnthropicStream(strings.NewReader(sse), func(delta string) { streamed = append(streamed, delta) })
	if err != nil {
		t.Fatalf("parseAnthropicStream() error = %v", err)
	}
	if result.Text != "Hello, world" {
		t.Errorf("Text = %q, want %q", result.Text, "Hello, world")
	}
	if result.StopReason != StopReasonEndTurn {
		t.Errorf("StopReason = %q, want %q", result.StopReason, StopReasonEndTurn)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("ToolCalls = %v, want none", result.ToolCalls)
	}
	if got := strings.Join(streamed, ""); got != "Hello, world" {
		t.Errorf("streamed deltas joined = %q, want %q", got, "Hello, world")
	}
}

func TestParseAnthropicStream_ToolUse(t *testing.T) {
	sse := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Let me check that."}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":0}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_1","name":"list_apps"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"pro"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"ject\":\"web\"}"}}`,
		``,
		`event: content_block_stop`,
		`data: {"type":"content_block_stop","index":1}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		``,
		`event: message_stop`,
		`data: {"type":"message_stop"}`,
		``,
	}, "\n")

	result, err := parseAnthropicStream(strings.NewReader(sse), nil)
	if err != nil {
		t.Fatalf("parseAnthropicStream() error = %v", err)
	}
	if result.Text != "Let me check that." {
		t.Errorf("Text = %q, want %q", result.Text, "Let me check that.")
	}
	if result.StopReason != StopReasonToolUse {
		t.Errorf("StopReason = %q, want %q", result.StopReason, StopReasonToolUse)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(result.ToolCalls))
	}
	call := result.ToolCalls[0]
	if call.ID != "toolu_1" || call.Name != "list_apps" {
		t.Errorf("call = %+v, want id=toolu_1 name=list_apps", call)
	}
	if string(call.Input) != `{"project":"web"}` {
		t.Errorf("call.Input = %s, want reassembled partial_json", call.Input)
	}
}

func TestParseAnthropicStream_MultipleToolCalls_OrderPreserved(t *testing.T) {
	sse := strings.Join([]string{
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"toolu_a","name":"get_app"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		``,
		`event: content_block_start`,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"toolu_b","name":"get_database"}}`,
		``,
		`event: content_block_delta`,
		`data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{}"}}`,
		``,
		`event: message_delta`,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"}}`,
		``,
	}, "\n")

	result, err := parseAnthropicStream(strings.NewReader(sse), nil)
	if err != nil {
		t.Fatalf("parseAnthropicStream() error = %v", err)
	}
	if len(result.ToolCalls) != 2 {
		t.Fatalf("len(ToolCalls) = %d, want 2", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ID != "toolu_a" || result.ToolCalls[1].ID != "toolu_b" {
		t.Errorf("ToolCalls order = [%s, %s], want [toolu_a, toolu_b] (stream order)", result.ToolCalls[0].ID, result.ToolCalls[1].ID)
	}
}

func TestParseAnthropicStream_ErrorEvent(t *testing.T) {
	sse := "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"model overloaded\"}}\n\n"
	_, err := parseAnthropicStream(strings.NewReader(sse), nil)
	if err == nil {
		t.Fatal("parseAnthropicStream() error = nil, want an error for an error event")
	}
	if !strings.Contains(err.Error(), "model overloaded") {
		t.Errorf("error = %v, want it to mention the api error message", err)
	}
}

func TestToJSONRawMessageAndToolResultText_RoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"plain text", "hello, this is not json"},
		{"json object", `{"apps":["web","api"]}`},
		{"empty string", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := toJSONRawMessage(tt.input)
			if !json.Valid(raw) {
				t.Fatalf("toJSONRawMessage(%q) = %s, not valid JSON", tt.input, raw)
			}
			if got := toolResultText(raw); got != tt.input {
				t.Errorf("toolResultText(toJSONRawMessage(%q)) = %q, want %q", tt.input, got, tt.input)
			}
		})
	}
}
