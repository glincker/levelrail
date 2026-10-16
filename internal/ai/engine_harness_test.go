package ai

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type errSink struct {
	fakeSink
	errs []TurnErrorInfo
}

func (s *errSink) TurnError(i TurnErrorInfo) { s.errs = append(s.errs, i) }

type errProvider struct {
	text string
	err  error
}

func (p errProvider) Complete(_ context.Context, _ ChatRequest, onText TextDeltaFunc) (TurnResult, error) {
	if p.text != "" && onText != nil {
		onText(p.text)
	}
	return TurnResult{Text: p.text}, p.err
}

func engineFor(db *store.DB, p Provider, tools ToolExecutor) *Engine {
	return NewEngine(db, &fakeSecrets{key: "k", exists: true}, tools, func(_, _ string) (Provider, error) { return p, nil }, "")
}

func TestEngine_HalfSucceededStreamKeepsSessionConsistent(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()
	sid := newTestSession(ctx, t, db)
	cut := &ProviderError{Provider: "anthropic", Class: ClassStreamCut}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "deploy_app"}}}
	e := engineFor(db, errProvider{text: "Looking at it", err: cut}, tools)

	sink := &errSink{}
	err := e.RunTurn(ctx, sid, "do it", sink)
	if !errors.Is(err, cut) {
		t.Fatalf("err = %v, want the stream cut", err)
	}
	if len(sink.errs) != 1 || sink.errs[0].Code != string(ClassStreamCut) || sink.errs[0].Fix == "" {
		t.Errorf("error events = %+v", sink.errs)
	}
	rows, _ := db.ListAIChatMessages(ctx, sid)
	if len(rows) != 2 || rows[1].Role != store.AIChatRoleAssistant || !strings.Contains(rows[1].Content, "interrupted") || len(rows[1].ToolCalls) != 0 {
		t.Fatalf("rows = %+v", rows)
	}
	if len(tools.calls) != 0 {
		t.Errorf("a cut stream must never execute tools: %v", tools.calls)
	}
	// The session is usable again: a retry runs on top of the saved history.
	fp := &fakeProvider{turns: []TurnResult{{Text: "ok", StopReason: StopReasonEndTurn}}}
	if err := engineFor(db, fp, tools).RunTurn(ctx, sid, "again", &fakeSink{}); err != nil {
		t.Errorf("retry after cut: %v", err)
	}
}

func TestEngine_NoTextSavedWhenNothingStreamed(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()
	sid := newTestSession(ctx, t, db)
	e := engineFor(db, errProvider{err: &ProviderError{Class: ClassInvalidKey, Provider: "anthropic"}}, &fakeToolExecutor{})
	sink := &errSink{}
	if err := e.RunTurn(ctx, sid, "hi", sink); err == nil {
		t.Fatal("want error")
	}
	rows, _ := db.ListAIChatMessages(ctx, sid)
	if len(rows) != 1 || sink.errs[0].Code != "invalid_key" {
		t.Errorf("rows = %d errs = %+v", len(rows), sink.errs)
	}
	// Consecutive user messages are merged for the provider.
	merged := mergeAdjacent([]Message{userText("a"), userText("b")})
	if len(merged) != 1 || len(merged[0].Content) != 2 {
		t.Errorf("merge = %+v", merged)
	}
}

func TestEngine_MaxStepsStopsLoop(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()
	sid := newTestSession(ctx, t, db)
	call := TurnResult{ToolCalls: []ToolUseCall{{ID: "t", Name: "list_apps", Input: json.RawMessage(`{}`)}}, StopReason: StopReasonToolUse}
	fp := &fakeProvider{turns: []TurnResult{call, call, call, call, call, call}}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "list_apps", Traits: ToolTraits{Known: true, ReadOnly: true}}}}
	e := engineFor(db, fp, tools)
	l := e.limits
	l.MaxSteps = 3
	e.SetLimits(l)

	sink := &errSink{}
	err := e.RunTurn(ctx, sid, "loop", sink)
	if !errors.Is(err, ErrMaxSteps) || fp.calls != 3 || len(sink.errs) != 1 || sink.errs[0].Code != "max_steps" {
		t.Errorf("err=%v calls=%d errs=%+v", err, fp.calls, sink.errs)
	}
}

func TestEngine_InvalidAndUnknownToolCallsNeverRun(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()
	sid := newTestSession(ctx, t, db)
	schema := json.RawMessage(`{"type":"object","required":["name"],"properties":{"name":{"type":"string"}}}`)
	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{
			{ID: "a", Name: "deploy_app", Input: json.RawMessage(`{"name":5}`)},
			{ID: "b", Name: "does_not_exist", Input: json.RawMessage(`{}`)},
		}, StopReason: StopReasonToolUse},
		{Text: "fixed", StopReason: StopReasonEndTurn},
	}}
	tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "deploy_app", InputSchema: schema}}}
	sink := &fakeSink{}
	if err := engineFor(db, fp, tools).RunTurn(ctx, sid, "go", sink); err != nil {
		t.Fatal(err)
	}
	if len(tools.calls) != 0 || len(sink.proposed) != 0 {
		t.Errorf("calls=%v proposed=%v, want none", tools.calls, sink.proposed)
	}
	if len(sink.results) != 2 || !sink.results[0].isError || !strings.Contains(string(sink.results[0].result), "invalid arguments") || !strings.Contains(string(sink.results[1].result), "unknown tool") {
		t.Errorf("results = %+v", sink.results)
	}
}

func TestEngine_ApprovalGateHoldsForEveryProviderKind(t *testing.T) {
	for _, kind := range []string{"anthropic", "openai_compat", "gemini"} {
		t.Run(kind, func(t *testing.T) {
			db := openTestDB(t)
			configureTestDB(t, db)
			ctx := context.Background()
			sid := newTestSession(ctx, t, db)
			var srvBody string
			switch kind {
			case "anthropic":
				srvBody = `data: {"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"t1","name":"deploy_app"}}` + "\n\n" +
					`data: {"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{}"}}` + "\n\n" +
					`data: {"type":"message_stop"}` + "\n\n"
			case "openai_compat":
				srvBody = `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"t1","function":{"name":"deploy_app","arguments":"{}"}}]},"finish_reason":"tool_calls"}]}` + "\n\ndata: [DONE]\n\n"
			default:
				srvBody = `data: {"candidates":[{"content":{"parts":[{"functionCall":{"name":"deploy_app","args":{}}}]},"finishReason":"STOP"}]}` + "\n\n"
			}
			srv := sseServer(t, 200, srvBody, nil)
			cfg := AdapterConfig{ProviderID: kind, BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client()}
			var p Provider
			switch kind {
			case "anthropic":
				p = NewAnthropic(cfg)
			case "openai_compat":
				p = NewOpenAICompat(cfg)
			default:
				p = NewGemini(cfg)
			}
			tools := &fakeToolExecutor{tools: []ToolSpec{{Name: "deploy_app"}}}
			sink := &fakeSink{}
			if err := engineFor(db, p, tools).RunTurn(ctx, sid, "deploy", sink); err != nil {
				t.Fatal(err)
			}
			if len(tools.calls) != 0 || len(sink.proposed) != 1 {
				t.Errorf("calls=%v proposed=%v: a mutating tool must wait for approval", tools.calls, sink.proposed)
			}
		})
	}
}

func TestEngine_HugeToolOutputTruncated(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()
	sid := newTestSession(ctx, t, db)
	fp := &fakeProvider{turns: []TurnResult{
		{ToolCalls: []ToolUseCall{{ID: "t", Name: "get_logs", Input: json.RawMessage(`{}`)}}, StopReason: StopReasonToolUse},
		{Text: "done", StopReason: StopReasonEndTurn},
	}}
	tools := &fakeToolExecutor{
		tools:   []ToolSpec{{Name: "get_logs", Traits: ToolTraits{Known: true, ReadOnly: true, OpenWorld: false}}},
		results: map[string]fakeToolResult{"get_logs": {text: strings.Repeat("L", 5<<20)}},
	}
	e := engineFor(db, fp, tools)
	l := e.limits
	l.ToolOutputMaxBytes = 1000
	e.SetLimits(l)
	sink := &fakeSink{}
	if err := e.RunTurn(ctx, sid, "logs", sink); err != nil {
		t.Fatal(err)
	}
	if len(sink.results) != 1 || len(sink.results[0].result) > 1200 || !strings.Contains(string(sink.results[0].result), "truncated") {
		t.Errorf("result len = %d", len(sink.results[0].result))
	}
}

type memUsage struct {
	mu   sync.Mutex
	recs []store.AIUsageRecord
	used int64
}

func (m *memUsage) RecordAIUsage(_ context.Context, r store.AIUsageRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recs = append(m.recs, r)
	return nil
}

func (m *memUsage) SumAITokensSince(_ context.Context, _ time.Time) (int64, error) {
	return m.used, nil
}

func TestEngine_BudgetBlocksNewTurns(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()
	sid := newTestSession(ctx, t, db)
	fp := &fakeProvider{turns: []TurnResult{{Text: "x", StopReason: StopReasonEndTurn}}}
	e := engineFor(db, fp, &fakeToolExecutor{})
	e.SetUsage(&memUsage{used: 1000})
	l := e.limits
	l.MonthlyTokenBudget = 1000
	e.SetLimits(l)

	sink := &errSink{}
	err := e.RunTurn(ctx, sid, "hi", sink)
	pe, ok := AsProviderError(err)
	if !ok || pe.Class != ClassBudgetExhausted || fp.calls != 0 {
		t.Fatalf("err=%v calls=%d", err, fp.calls)
	}
	if len(sink.errs) != 1 || !strings.Contains(sink.errs[0].Message, "1000 of 1000") {
		t.Errorf("errs = %+v", sink.errs)
	}
	if rows, _ := db.ListAIChatMessages(ctx, sid); len(rows) != 0 {
		t.Error("a blocked turn must not store the user message")
	}
	l.MonthlyTokenBudget = 0
	e.SetLimits(l)
	if err := e.RunTurn(ctx, sid, "hi", &fakeSink{}); err != nil {
		t.Errorf("no budget should not block: %v", err)
	}
}

type chainResolver struct{ p Provider }

func (c chainResolver) Configured(context.Context) (bool, error) { return true, nil }
func (c chainResolver) Chain(context.Context) (Provider, error)  { return c.p, nil }

func TestEngine_RecordsUsagePerAttempt(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	sid := newTestSession(ctx, t, db)
	chain := &ChainProvider{DefaultContext: 100000, Entries: []ChainEntry{
		{Provider: "a", Model: "ma", Backend: &scriptedProvider{errs: []error{&ProviderError{Class: ClassRateLimited}}}},
		{Provider: "b", Model: "mb", Backend: &scriptedProvider{}},
	}}
	e := NewEngine(db, &fakeSecrets{}, &fakeToolExecutor{}, nil, "")
	e.SetProviders(chainResolver{chain})
	usage := &memUsage{}
	e.SetUsage(usage)
	if err := e.RunTurn(ctx, sid, "hi", &fakeSink{}); err != nil {
		t.Fatal(err)
	}
	if len(usage.recs) != 2 || usage.recs[0].ErrorClass != "rate_limited" || usage.recs[1].Provider != "b" || usage.recs[1].SessionID != sid || usage.recs[1].MessageID == "" {
		t.Errorf("recs = %+v", usage.recs)
	}
}

func TestEngine_ConcurrentTurnsAcrossSessions(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		sid := newTestSession(ctx, t, db)
		wg.Add(1)
		go func() {
			defer wg.Done()
			fp := &fakeProvider{turns: []TurnResult{{Text: "ok", StopReason: StopReasonEndTurn}}}
			errs <- engineFor(db, fp, &fakeToolExecutor{}).RunTurn(ctx, sid, "hi", &fakeSink{})
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent turn: %v", err)
		}
	}
}

func TestStepTimeoutCancelsSlowProvider(t *testing.T) {
	db := openTestDB(t)
	configureTestDB(t, db)
	ctx := context.Background()
	sid := newTestSession(ctx, t, db)
	slow := blockingProvider{}
	e := engineFor(db, slow, &fakeToolExecutor{})
	l := e.limits
	l.StepTimeout = 50 * time.Millisecond
	e.SetLimits(l)
	sink := &errSink{}
	if err := e.RunTurn(ctx, sid, "hi", sink); err == nil || len(sink.errs) != 1 {
		t.Errorf("err=%v errs=%v", err, sink.errs)
	}
}

type blockingProvider struct{}

func (blockingProvider) Complete(ctx context.Context, _ ChatRequest, _ TextDeltaFunc) (TurnResult, error) {
	<-ctx.Done()
	return TurnResult{}, ClassifyTransport("slow", ctx.Err(), "")
}
