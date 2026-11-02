package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

// inputRecordingTools records the raw input each Call receives.
type inputRecordingTools struct {
	fakeToolExecutor
	inputs []string
}

func (r *inputRecordingTools) Call(ctx context.Context, name string, in json.RawMessage) (string, bool, error) {
	r.inputs = append(r.inputs, string(in))
	return r.fakeToolExecutor.Call(ctx, name, in)
}

func TestEngine_ApproveExecutesRealSecretThenStoresMasked(t *testing.T) {
	secret := "sk-engine-approve-secret-98765" //nolint:gosec // synthetic test value
	secretmask.Add(secret)
	db := openTestDB(t)
	ctx := context.Background()
	sessionID := newTestSession(ctx, t, db)

	input := json.RawMessage(`{"key":"API_KEY","value":"` + secret + `"}`)
	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "set_app_env", Input: input}}, StopReason: StopReasonToolUse},
		{Text: "done", StopReason: StopReasonEndTurn},
	}}
	tools := &inputRecordingTools{fakeToolExecutor: fakeToolExecutor{
		tools:   []ToolSpec{{Name: "set_app_env"}},
		results: map[string]fakeToolResult{"set_app_env": {text: `{"ok":true}`}},
	}}
	engine := NewEngine(db, staticChain{p: fp}, tools, "")

	sink := &fakeSink{}
	if err := engine.RunTurn(ctx, sessionID, "set it", sink); err != nil {
		t.Fatal(err)
	}
	confID := sink.proposed[0].confirmationID

	pending, err := db.GetAIChatConfirmation(ctx, confID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pending.ToolInput), secret) {
		t.Fatalf("pending input must keep the real value, got %s", pending.ToolInput)
	}

	if err := engine.ResolveConfirmation(ctx, sessionID, confID, true, &fakeSink{}); err != nil {
		t.Fatal(err)
	}
	if len(tools.inputs) != 1 || !strings.Contains(tools.inputs[0], secret) || strings.Contains(tools.inputs[0], secretmask.Marker) {
		t.Fatalf("tool must run with the real value, got %v", tools.inputs)
	}

	done, err := db.GetAIChatConfirmation(ctx, confID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(done.ToolInput), secret) || !strings.Contains(string(done.ToolInput), "API_KEY") {
		t.Errorf("resolved input must be masked, got %s", done.ToolInput)
	}
}
