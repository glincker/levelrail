package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteGuardRefusesMutatingInObserveMode(t *testing.T) {
	e := &Engine{}
	e.SetModeSource(func(context.Context) string { return "observe" })
	if got := e.writeRefusal(context.Background(), ToolTraits{Known: true}, "deploy_app"); !strings.Contains(got, "observe") {
		t.Fatalf("mutating call not refused: %q", got)
	}
	if got := e.writeRefusal(context.Background(), ToolTraits{Known: true, ReadOnly: true}, "list_apps"); got != "" {
		t.Fatalf("read-only call refused: %q", got)
	}
	e.SetModeSource(func(context.Context) string { return "operate" })
	if got := e.writeRefusal(context.Background(), ToolTraits{Known: true}, "deploy_app"); got != "" {
		t.Fatalf("operate mode refused: %q", got)
	}
}

func TestGuardedToolsStopsIdenticalRepeats(t *testing.T) {
	f := &fakeToolExecutor{results: map[string]fakeToolResult{"logs": {text: "line"}}}
	g := newGuardedTools(f, Limits{RepeatLimit: 2})
	args := json.RawMessage(`{"app":"web"}`)
	for range 2 {
		if _, isErr, _ := g.Call(context.Background(), "logs", args); isErr {
			t.Fatal("early call errored")
		}
	}
	text, isErr, _ := g.Call(context.Background(), "logs", args)
	if !isErr || !strings.Contains(text, "Stop calling it") || len(f.calls) != 2 {
		t.Fatalf("third call not blocked: %q err=%v calls=%d", text, isErr, len(f.calls))
	}
}

func TestGuardedToolsTellsModelToStopAfterEmpties(t *testing.T) {
	f := &fakeToolExecutor{results: map[string]fakeToolResult{"search": {text: "[]"}}}
	g := newGuardedTools(f, Limits{EmptyLimit: 2})
	_, _, _ = g.Call(context.Background(), "search", json.RawMessage(`{"q":"a"}`))
	text, _, _ := g.Call(context.Background(), "search", json.RawMessage(`{"q":"b"}`))
	if !strings.Contains(text, "Stop retrying") {
		t.Fatalf("no stop note after repeated empties: %q", text)
	}
}

func TestScrubToolErrorRemovesSecretsPathsAndTraces(t *testing.T) {
	in := "open /Users/ops/levelrail/data/db.sqlite: denied Authorization: Bearer abcdef1234567890abcdef token=sk-abcdefgh12345678\ngoroutine 5 [running]:\nmain.go:12 +0x1"
	got := ScrubToolError(in)
	for _, bad := range []string{"/Users/ops", "abcdef1234567890", "sk-abcdefgh", "goroutine"} {
		if strings.Contains(got, bad) {
			t.Fatalf("%q leaked in %q", bad, got)
		}
	}
}
