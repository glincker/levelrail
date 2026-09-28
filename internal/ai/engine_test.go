package ai

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func openTestDB(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "levelrail.db")
	db, err := store.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("store.Open(%q) error = %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func configureTestDB(t *testing.T, db *store.DB) {
	t.Helper()
	if err := db.UpdateAIAssistantSettings(context.Background(), store.AIAssistantSettings{
		Provider: store.AIProviderAnthropic, Model: "claude-test",
	}); err != nil {
		t.Fatalf("UpdateAIAssistantSettings() error = %v", err)
	}
}

// newTestSession creates a chat session row under ctx, for tests that
// need a valid session id but don't exercise session creation itself.
func newTestSession(ctx context.Context, t *testing.T, db *store.DB) string {
	t.Helper()
	sessionID, err := store.NewAIChatSessionID()
	if err != nil {
		t.Fatalf("NewAIChatSessionID() error = %v", err)
	}
	if _, err := db.CreateAIChatSession(ctx, sessionID, time.Now()); err != nil {
		t.Fatalf("CreateAIChatSession() error = %v", err)
	}
	return sessionID
}

// fakeProvider replays a fixed script of turns, one per Complete call, so
// tests can exercise the engine's propose -> pause -> confirm -> continue
// loop without a real network call.
type fakeProvider struct {
	turns []TurnResult
	calls int
}

func (f *fakeProvider) Complete(_ context.Context, _ ChatRequest, onText TextDeltaFunc) (TurnResult, error) {
	if f.calls >= len(f.turns) {
		return TurnResult{}, errors.New("fakeProvider: no more scripted turns")
	}
	turn := f.turns[f.calls]
	f.calls++
	if onText != nil && turn.Text != "" {
		onText(turn.Text)
	}
	return turn, nil
}

func fakeProviderFactory(fp *fakeProvider) ProviderFactory {
	return func(_, _ string) (Provider, error) { return fp, nil }
}

// fakeToolExecutor is a scripted ToolExecutor: Call returns a canned
// result per tool name, and every call is recorded for assertions.
type fakeToolExecutor struct {
	tools   []ToolSpec
	results map[string]fakeToolResult
	calls   []string
}

type fakeToolResult struct {
	text    string
	isError bool
}

func (f *fakeToolExecutor) ListTools(_ context.Context) ([]ToolSpec, error) { return f.tools, nil }

func (f *fakeToolExecutor) Call(_ context.Context, name string, _ json.RawMessage) (string, bool, error) {
	f.calls = append(f.calls, name)
	r, ok := f.results[name]
	if !ok {
		return `{}`, false, nil
	}
	return r.text, r.isError, nil
}

// fakeSecrets is a scripted SecretsResolver: a single, fixed key, either
// present or absent.
type fakeSecrets struct {
	key    string
	exists bool
}

func (f *fakeSecrets) Resolve(_ context.Context, _, _ string) (string, error) {
	if !f.exists {
		return "", errors.New("fakeSecrets: not found")
	}
	return f.key, nil
}

func (f *fakeSecrets) Exists(_ context.Context, _, _ string) (bool, error) { return f.exists, nil }

type proposedCall struct {
	confirmationID, toolUseID, toolName string
	arguments                           json.RawMessage
}

type resultCall struct {
	toolUseID, toolName string
	result              json.RawMessage
	isError             bool
}

// fakeSink records every event Engine emits, for assertion.
type fakeSink struct {
	texts    []string
	proposed []proposedCall
	results  []resultCall
}

func (f *fakeSink) TextDelta(text string) { f.texts = append(f.texts, text) }

func (f *fakeSink) ToolCallProposed(confirmationID, toolUseID, toolName string, arguments json.RawMessage) {
	f.proposed = append(f.proposed, proposedCall{confirmationID, toolUseID, toolName, arguments})
}

func (f *fakeSink) ToolResult(toolUseID, toolName string, result json.RawMessage, isError bool) {
	f.results = append(f.results, resultCall{toolUseID, toolName, result, isError})
}

func TestEngine_RunTurn_NoToolCalls(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()
	sessionID := newTestSession(ctx, t, db)

	fp := &fakeProvider{turns: []TurnResult{{Text: "hello there", StopReason: StopReasonEndTurn}}}
	engine := NewEngine(db, &fakeSecrets{key: "sk-test", exists: true}, &fakeToolExecutor{}, fakeProviderFactory(fp), "")

	sink := &fakeSink{}
	if err := engine.RunTurn(ctx, sessionID, "hi", sink); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if got := joinTexts(sink.texts); got != "hello there" {
		t.Errorf("streamed text = %q, want %q", got, "hello there")
	}
	if len(sink.proposed) != 0 || len(sink.results) != 0 {
		t.Errorf("expected no proposed/result events, got proposed=%v results=%v", sink.proposed, sink.results)
	}

	msgs, err := db.ListAIChatMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListAIChatMessages() error = %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("len(msgs) = %d, want 2 (user + assistant)", len(msgs))
	}
	if msgs[0].Role != store.AIChatRoleUser || msgs[0].Content != "hi" {
		t.Errorf("msgs[0] = %+v, want user/hi", msgs[0])
	}
	if msgs[1].Role != store.AIChatRoleAssistant || msgs[1].Content != "hello there" {
		t.Errorf("msgs[1] = %+v, want assistant/hello there", msgs[1])
	}
}

func TestEngine_RunTurn_AutoExecutesReadOnlyTool(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()

	sessionID := newTestSession(ctx, t, db)

	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "list_apps", Input: json.RawMessage(`{}`)}}, StopReason: StopReasonToolUse},
		{Text: "you have 2 apps", StopReason: StopReasonEndTurn},
	}}
	tools := &fakeToolExecutor{
		tools:   []ToolSpec{{Name: "list_apps", Description: "list apps", Traits: ToolTraits{Known: true, ReadOnly: true}}},
		results: map[string]fakeToolResult{"list_apps": {text: `[{"name":"web"},{"name":"api"}]`}},
	}
	engine := NewEngine(db, &fakeSecrets{key: "sk-test", exists: true}, tools, fakeProviderFactory(fp), "")

	sink := &fakeSink{}
	if err := engine.RunTurn(ctx, sessionID, "how many apps", sink); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(sink.proposed) != 0 {
		t.Errorf("expected no proposed confirmations for a read-only tool, got %v", sink.proposed)
	}
	if len(sink.results) != 1 || sink.results[0].toolName != "list_apps" {
		t.Fatalf("results = %v, want one list_apps result", sink.results)
	}
	if len(tools.calls) != 1 || tools.calls[0] != "list_apps" {
		t.Errorf("tools.calls = %v, want [list_apps]", tools.calls)
	}

	msgs, err := db.ListAIChatMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListAIChatMessages() error = %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3 (user, assistant-with-tool-call, assistant-final)", len(msgs))
	}
	if len(msgs[1].ToolCalls) != 1 || msgs[1].ToolCalls[0].Status != "auto_executed" {
		t.Errorf("msgs[1].ToolCalls = %+v, want one auto_executed call", msgs[1].ToolCalls)
	}
	if msgs[2].Content != "you have 2 apps" {
		t.Errorf("msgs[2].Content = %q, want final reply", msgs[2].Content)
	}
}

func TestEngine_RunTurn_ProposesMutatingTool_PausesForConfirmation(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()

	sessionID := newTestSession(ctx, t, db)

	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "deploy_app", Input: json.RawMessage(`{"name":"web","image":"web:2"}`)}}, StopReason: StopReasonToolUse},
	}}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "deploy_app"}}}
	engine := NewEngine(db, &fakeSecrets{key: "sk-test", exists: true}, tools, fakeProviderFactory(fp), "")

	sink := &fakeSink{}
	if err := engine.RunTurn(ctx, sessionID, "deploy web to 2", sink); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	if len(sink.results) != 0 {
		t.Errorf("expected no auto-executed results for a mutating tool, got %v", sink.results)
	}
	if len(sink.proposed) != 1 || sink.proposed[0].toolName != "deploy_app" {
		t.Fatalf("proposed = %v, want one deploy_app proposal", sink.proposed)
	}
	if len(tools.calls) != 0 {
		t.Errorf("tools.calls = %v, want no calls: a mutating tool must never auto-execute", tools.calls)
	}

	confirmationID := sink.proposed[0].confirmationID
	conf, err := db.GetAIChatConfirmation(ctx, confirmationID)
	if err != nil {
		t.Fatalf("GetAIChatConfirmation() error = %v", err)
	}
	if conf.Status != store.AIChatConfirmationPending {
		t.Errorf("confirmation status = %q, want pending", conf.Status)
	}

	// A second user message must be refused while this turn's mutating
	// call is still unresolved: sending it would produce a conversation
	// missing a tool_result the model already asked for.
	if err := engine.RunTurn(ctx, sessionID, "actually cancel that", &fakeSink{}); !errors.Is(err, ErrPendingConfirmation) {
		t.Errorf("RunTurn() while pending, error = %v, want ErrPendingConfirmation", err)
	}
}

func TestEngine_ResolveConfirmation_ApproveContinuesTurn(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()

	sessionID := newTestSession(ctx, t, db)

	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "deploy_app", Input: json.RawMessage(`{}`)}}, StopReason: StopReasonToolUse},
		{Text: "deployed successfully", StopReason: StopReasonEndTurn},
	}}
	tools := &fakeToolExecutor{
		tools:   []ToolSpec{{Name: "deploy_app"}},
		results: map[string]fakeToolResult{"deploy_app": {text: `{"status":"ok"}`}},
	}
	engine := NewEngine(db, &fakeSecrets{key: "sk-test", exists: true}, tools, fakeProviderFactory(fp), "")

	proposeSink := &fakeSink{}
	if err := engine.RunTurn(ctx, sessionID, "deploy web", proposeSink); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	confirmationID := proposeSink.proposed[0].confirmationID

	resolveSink := &fakeSink{}
	if err := engine.ResolveConfirmation(ctx, sessionID, confirmationID, true, resolveSink); err != nil {
		t.Fatalf("ResolveConfirmation() error = %v", err)
	}

	if len(resolveSink.results) != 1 || resolveSink.results[0].isError {
		t.Fatalf("resolveSink.results = %v, want one successful result", resolveSink.results)
	}
	if got := joinTexts(resolveSink.texts); got != "deployed successfully" {
		t.Errorf("resolveSink text = %q, want continuation text", got)
	}
	if len(tools.calls) != 1 || tools.calls[0] != "deploy_app" {
		t.Errorf("tools.calls = %v, want exactly one deploy_app call, now that it's approved", tools.calls)
	}

	conf, err := db.GetAIChatConfirmation(ctx, confirmationID)
	if err != nil {
		t.Fatalf("GetAIChatConfirmation() error = %v", err)
	}
	if conf.Status != store.AIChatConfirmationApproved {
		t.Errorf("confirmation status = %q, want approved", conf.Status)
	}

	msgs, err := db.ListAIChatMessages(ctx, sessionID)
	if err != nil {
		t.Fatalf("ListAIChatMessages() error = %v", err)
	}
	if len(msgs) != 3 {
		t.Fatalf("len(msgs) = %d, want 3 (user, assistant-proposing, assistant-final)", len(msgs))
	}
	if msgs[1].ToolCalls[0].Status != store.AIChatConfirmationApproved {
		t.Errorf("msgs[1].ToolCalls[0].Status = %q, want approved (updated in place)", msgs[1].ToolCalls[0].Status)
	}
}

func TestEngine_ResolveConfirmation_Reject(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()

	sessionID := newTestSession(ctx, t, db)

	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "restart_app", Input: json.RawMessage(`{}`)}}, StopReason: StopReasonToolUse},
		{Text: "ok, left it running", StopReason: StopReasonEndTurn},
	}}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "restart_app"}}}
	engine := NewEngine(db, &fakeSecrets{key: "sk-test", exists: true}, tools, fakeProviderFactory(fp), "")

	proposeSink := &fakeSink{}
	if err := engine.RunTurn(ctx, sessionID, "restart web", proposeSink); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	confirmationID := proposeSink.proposed[0].confirmationID

	resolveSink := &fakeSink{}
	if err := engine.ResolveConfirmation(ctx, sessionID, confirmationID, false, resolveSink); err != nil {
		t.Fatalf("ResolveConfirmation() error = %v", err)
	}

	if len(tools.calls) != 0 {
		t.Errorf("tools.calls = %v, want no calls: a rejected confirmation must never execute", tools.calls)
	}
	if len(resolveSink.results) != 1 || !resolveSink.results[0].isError {
		t.Fatalf("resolveSink.results = %v, want one error-flagged rejection result", resolveSink.results)
	}

	conf, err := db.GetAIChatConfirmation(ctx, confirmationID)
	if err != nil {
		t.Fatalf("GetAIChatConfirmation() error = %v", err)
	}
	if conf.Status != store.AIChatConfirmationRejected {
		t.Errorf("confirmation status = %q, want rejected", conf.Status)
	}
}

func TestEngine_ResolveConfirmation_AlreadyResolved(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()

	sessionID := newTestSession(ctx, t, db)

	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "restart_app", Input: json.RawMessage(`{}`)}}, StopReason: StopReasonToolUse},
		{Text: "done", StopReason: StopReasonEndTurn},
	}}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "restart_app"}}}
	engine := NewEngine(db, &fakeSecrets{key: "sk-test", exists: true}, tools, fakeProviderFactory(fp), "")

	proposeSink := &fakeSink{}
	_ = engine.RunTurn(ctx, sessionID, "restart web", proposeSink)
	confirmationID := proposeSink.proposed[0].confirmationID

	if err := engine.ResolveConfirmation(ctx, sessionID, confirmationID, true, &fakeSink{}); err != nil {
		t.Fatalf("first ResolveConfirmation() error = %v", err)
	}
	if err := engine.ResolveConfirmation(ctx, sessionID, confirmationID, true, &fakeSink{}); !errors.Is(err, ErrConfirmationAlreadyResolved) {
		t.Errorf("second ResolveConfirmation() error = %v, want ErrConfirmationAlreadyResolved", err)
	}
}

func TestEngine_ResolveConfirmation_WrongSession(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()

	sessionA := newTestSession(ctx, t, db)
	sessionB := newTestSession(ctx, t, db)

	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t1", Name: "restart_app", Input: json.RawMessage(`{}`)}}, StopReason: StopReasonToolUse},
	}}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "restart_app"}}}
	engine := NewEngine(db, &fakeSecrets{key: "sk-test", exists: true}, tools, fakeProviderFactory(fp), "")

	proposeSink := &fakeSink{}
	_ = engine.RunTurn(ctx, sessionA, "restart web", proposeSink)
	confirmationID := proposeSink.proposed[0].confirmationID

	if err := engine.ResolveConfirmation(ctx, sessionB, confirmationID, true, &fakeSink{}); !errors.Is(err, ErrConfirmationSessionMismatch) {
		t.Errorf("ResolveConfirmation() from wrong session, error = %v, want ErrConfirmationSessionMismatch", err)
	}
}

func TestEngine_IsConfigured(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	engine := NewEngine(db, &fakeSecrets{exists: false}, &fakeToolExecutor{}, nil, "")
	ok, err := engine.IsConfigured(ctx)
	if err != nil {
		t.Fatalf("IsConfigured() error = %v", err)
	}
	if ok {
		t.Error("IsConfigured() = true before provider/model/key are set, want false")
	}

	configureTestDB(t, db)
	engineWithKey := NewEngine(db, &fakeSecrets{key: "sk-test", exists: true}, &fakeToolExecutor{}, nil, "")
	ok, err = engineWithKey.IsConfigured(ctx)
	if err != nil {
		t.Fatalf("IsConfigured() error = %v", err)
	}
	if !ok {
		t.Error("IsConfigured() = false after provider/model/key are set, want true")
	}
}

func joinTexts(texts []string) string {
	out := ""
	for _, t := range texts {
		out += t
	}
	return out
}
