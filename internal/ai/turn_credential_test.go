package ai

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
)

type attributedSink struct {
	fakeSink
	attrs []ToolAttribution
}

func (s *attributedSink) ToolResultAttributed(toolUseID, toolName string, result json.RawMessage, isError bool, attr ToolAttribution) {
	s.ToolResult(toolUseID, toolName, result, isError)
	s.attrs = append(s.attrs, attr)
}

func turnFactoryEngine(t *testing.T, fp *fakeProvider, tools *fakeToolExecutor, tokens *[]string) (*Engine, string) {
	t.Helper()
	db := openTestDB(t)
	sessionID := newTestSession(context.Background(), t, db)
	engine := NewEngine(db, staticChain{p: fp}, nil, "")
	engine.SetToolFactory(func(_ context.Context, token string) (ToolExecutor, error) {
		*tokens = append(*tokens, token)
		return tools, nil
	})
	return engine, sessionID
}

func TestEngine_ToolFactory_RefusesTurnWithoutCredential(t *testing.T) {
	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "list_apps", Input: json.RawMessage(`{}`)}}, StopReason: StopReasonToolUse},
	}}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "list_apps", Traits: ToolTraits{Known: true, ReadOnly: true}}}}
	var tokens []string
	engine, sessionID := turnFactoryEngine(t, fp, tools, &tokens)

	err := engine.RunTurn(context.Background(), sessionID, "how many apps", &fakeSink{})
	if !errors.Is(err, ErrNoTurnCredential) {
		t.Fatalf("RunTurn() error = %v, want ErrNoTurnCredential", err)
	}
	if len(tools.calls) != 0 || len(tokens) != 0 || fp.calls != 0 {
		t.Errorf("tool calls %v, tokens %v, model calls %d: want nothing to run without a turn credential", tools.calls, tokens, fp.calls)
	}
}

func TestEngine_ToolFactory_RunsToolsWithTurnTokenAndRecordsActor(t *testing.T) {
	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "list_apps", Input: json.RawMessage(`{}`)}}, StopReason: StopReasonToolUse},
		{Text: "two apps", StopReason: StopReasonEndTurn},
	}}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "list_apps", Traits: ToolTraits{Known: true, ReadOnly: true}}}}
	var tokens []string
	engine, sessionID := turnFactoryEngine(t, fp, tools, &tokens)

	ctx := WithTurnCredential(context.Background(), TurnCredential{
		Token: "turn-token", ActingAs: "alice@example.com",
		AbilitiesUsed: func() []string { return []string{"read"} },
	})
	sink := &attributedSink{}
	if err := engine.RunTurn(ctx, sessionID, "how many apps", sink); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if !slices.Equal(tokens, []string{"turn-token"}) {
		t.Errorf("factory tokens = %v, want exactly the turn token", tokens)
	}
	if len(sink.attrs) != 1 || sink.attrs[0].ActingAs != "alice@example.com" || !slices.Equal(sink.attrs[0].Abilities, []string{"read"}) {
		t.Errorf("sink attribution = %+v, want alice@example.com with read", sink.attrs)
	}
	msgs, err := engine.store.ListAIChatMessages(ctx, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	call := msgs[1].ToolCalls[0]
	if call.ActingAs != "alice@example.com" || !slices.Equal(call.Abilities, []string{"read"}) {
		t.Errorf("stored tool call = %+v, want acting_as and abilities recorded", call)
	}
}

func TestEngine_ToolFactory_ConfirmationRunsAsApprover(t *testing.T) {
	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "restart_app", Input: json.RawMessage(`{"name":"web"}`)}}, StopReason: StopReasonToolUse},
		{Text: "restarted", StopReason: StopReasonEndTurn},
	}}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "restart_app", Traits: ToolTraits{Known: true}}}}
	var tokens []string
	engine, sessionID := turnFactoryEngine(t, fp, tools, &tokens)

	propose := WithTurnCredential(context.Background(), TurnCredential{Token: "turn-1", ActingAs: "alice@example.com"})
	sink := &attributedSink{}
	if err := engine.RunTurn(propose, sessionID, "restart web", sink); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if len(sink.proposed) != 1 || len(tools.calls) != 0 {
		t.Fatalf("proposed %v calls %v, want one pending proposal and no execution", sink.proposed, tools.calls)
	}
	approve := WithTurnCredential(context.Background(), TurnCredential{
		Token: "turn-2", ActingAs: "alice@example.com",
		AbilitiesUsed: func() []string { return []string{"write"} },
	})
	if err := engine.ResolveConfirmation(approve, sessionID, sink.proposed[0].confirmationID, true, sink); err != nil {
		t.Fatalf("ResolveConfirmation() error = %v", err)
	}
	if !slices.Contains(tokens, "turn-2") || tokens[len(tokens)-1] != "turn-2" {
		t.Errorf("factory tokens = %v, want the approval's own turn token last", tokens)
	}
	if len(sink.attrs) != 1 || sink.attrs[0].ConfirmationID != sink.proposed[0].confirmationID || !slices.Equal(sink.attrs[0].Abilities, []string{"write"}) {
		t.Errorf("attribution = %+v, want the confirmation id and write", sink.attrs)
	}
}
