package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestPrintAIChatEventHuman_ToolResultShowsActingAs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	sawText := false
	printAIChatEventHuman(&stdout, &stderr, apiclient.AIChatSSEEvent{
		Type: "tool_result", Name: "restart_app", Result: json.RawMessage(`{}`),
		ActingAs: "alice@example.com", Abilities: []string{"write"},
	}, &sawText)
	if got := stderr.String(); !strings.Contains(got, "restart_app") || !strings.Contains(got, "acting as alice@example.com") || !strings.Contains(got, "write") {
		t.Errorf("stderr = %q, want the tool, the human it ran as and the ability", got)
	}
}

func TestPrintAISessionHuman_ToolCallShowsActingAs(t *testing.T) {
	var out bytes.Buffer
	printAISessionHuman(&out, apiclient.AIChatSessionResource{ID: "s1", Messages: []apiclient.AIChatMessageResource{{
		Role: "assistant",
		ToolCalls: []apiclient.AIChatToolCallResource{{
			Name: "list_apps", Status: "auto_executed", ActingAs: "alice@example.com", Abilities: []string{"read"},
		}},
	}}})
	if got := out.String(); !strings.Contains(got, "acting as alice@example.com") || !strings.Contains(got, "read") {
		t.Errorf("transcript = %q, want acting as and ability", got)
	}
}
