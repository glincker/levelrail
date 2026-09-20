package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeAIEngine is a scripted AIEngine, so handler tests can control the
// turn loop's behavior without a real internal/ai.Engine or Provider.
type fakeAIEngine struct {
	configured    bool
	configuredErr error
	runTurn       func(ctx context.Context, sessionID, userContent string, sink ai.Sink) error
	resolve       func(ctx context.Context, sessionID, confirmationID string, approve bool, sink ai.Sink) error
}

func (f *fakeAIEngine) IsConfigured(context.Context) (bool, error) {
	return f.configured, f.configuredErr
}

func (f *fakeAIEngine) RunTurn(ctx context.Context, sessionID, userContent string, sink ai.Sink) error {
	if f.runTurn != nil {
		return f.runTurn(ctx, sessionID, userContent, sink)
	}
	return nil
}

func (f *fakeAIEngine) ResolveConfirmation(ctx context.Context, sessionID, confirmationID string, approve bool, sink ai.Sink) error {
	if f.resolve != nil {
		return f.resolve(ctx, sessionID, confirmationID, approve, sink)
	}
	return nil
}

func newTestRouterWithAIEngine(t *testing.T, engine AIEngine) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithAIEngine(engine)), db
}

// newAIChatSession creates a chat session row under a background
// context, for tests that need a valid session id but don't exercise
// session creation itself.
func newAIChatSession(t *testing.T, db *store.DB) (context.Context, string) {
	t.Helper()
	ctx := context.Background()
	return ctx, mustCreateAIChatSession(ctx, t, db)
}

func mustCreateAIChatSession(ctx context.Context, t *testing.T, db *store.DB) string {
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

// seedPendingConfirmation saves an assistant message plus one tool
// confirmation under it, at the given status, for tests exercising the
// resolve-confirmation route against an already-persisted call.
func seedPendingConfirmation(ctx context.Context, t *testing.T, db *store.DB, sessionID, status string) string {
	t.Helper()
	msgID, err := store.NewAIChatMessageID()
	if err != nil {
		t.Fatalf("NewAIChatMessageID() error = %v", err)
	}
	if err := db.SaveAIChatMessage(ctx, store.AIChatMessage{
		ID: msgID, SessionID: sessionID, Role: store.AIChatRoleAssistant, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveAIChatMessage() error = %v", err)
	}
	confID, err := store.NewAIChatConfirmationID()
	if err != nil {
		t.Fatalf("NewAIChatConfirmationID() error = %v", err)
	}
	if err := db.SaveAIChatConfirmation(ctx, store.AIChatConfirmation{
		ID: confID, SessionID: sessionID, MessageID: msgID, ToolUseID: "t1", ToolName: "restart_app",
		ToolInput: json.RawMessage(`{}`), Status: status, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveAIChatConfirmation() error = %v", err)
	}
	return confID
}

// sseDataLines extracts every "data: <json>" payload from an SSE
// response body, in order, skipping the leading ": connected" priming
// comment line.
func sseDataLines(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if payload, ok := strings.CutPrefix(line, "data: "); ok {
			out = append(out, payload)
		}
	}
	return out
}

func sseEventTypes(t *testing.T, body string) []string {
	t.Helper()
	var types []string
	for _, data := range sseDataLines(body) {
		var ev struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			t.Fatalf("decode sse event %q: %v", data, err)
		}
		types = append(types, ev.Type)
	}
	return types
}

func TestHandleCreateAIChatSession(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions", ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var got aiChatSessionCreatedResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" {
		t.Error("ID is empty, want a minted session id")
	}

	if _, err := db.GetAIChatSession(context.Background(), got.ID); err != nil {
		t.Errorf("GetAIChatSession(%q) error = %v, want it to exist", got.ID, err)
	}
}

func TestHandleGetAIChatSession_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/ai/sessions/does-not-exist", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleGetAIChatSession_ReturnsHistory(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	sessionID := mustCreateAIChatSession(ctx, t, db)
	msgID, _ := store.NewAIChatMessageID()
	if err := db.SaveAIChatMessage(ctx, store.AIChatMessage{
		ID: msgID, SessionID: sessionID, Role: store.AIChatRoleUser, Content: "hello", CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("SaveAIChatMessage() error = %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/ai/sessions/"+sessionID, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got aiChatSessionResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID != sessionID || len(got.Messages) != 1 || got.Messages[0].Content != "hello" {
		t.Errorf("got %+v, want one message with content hello", got)
	}
}

func TestHandleCreateAIChatMessage_NoEngineConfigured(t *testing.T) {
	rt, db := newTestRouter(t) // no WithAIEngine
	cookie := loginTestSession(t, rt, db)
	_, sessionID := newAIChatSession(t, db)

	rec := httptest.NewRecorder()
	body := `{"content":"hi"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions/"+sessionID+"/messages", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
	}
}

func TestHandleCreateAIChatMessage_SessionNotFound(t *testing.T) {
	rt, db := newTestRouterWithAIEngine(t, &fakeAIEngine{configured: true})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	body := `{"content":"hi"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions/does-not-exist/messages", body))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleCreateAIChatMessage_NotConfigured(t *testing.T) {
	rt, db := newTestRouterWithAIEngine(t, &fakeAIEngine{configured: false})
	cookie := loginTestSession(t, rt, db)
	_, sessionID := newAIChatSession(t, db)

	rec := httptest.NewRecorder()
	body := `{"content":"hi"}`
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions/"+sessionID+"/messages", body))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
}

func TestHandleCreateAIChatMessage_RequiresContent(t *testing.T) {
	rt, db := newTestRouterWithAIEngine(t, &fakeAIEngine{configured: true})
	cookie := loginTestSession(t, rt, db)
	_, sessionID := newAIChatSession(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions/"+sessionID+"/messages", `{"content":""}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestHandleCreateAIChatMessage_StreamsTextThenDone(t *testing.T) {
	engine := &fakeAIEngine{
		configured: true,
		runTurn: func(_ context.Context, _, userContent string, sink ai.Sink) error {
			sink.TextDelta("hello ")
			sink.TextDelta("there")
			if userContent != "hi" {
				t.Errorf("userContent = %q, want hi", userContent)
			}
			return nil
		},
	}
	rt, db := newTestRouterWithAIEngine(t, engine)
	cookie := loginTestSession(t, rt, db)
	_, sessionID := newAIChatSession(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions/"+sessionID+"/messages", `{"content":"hi"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	types := sseEventTypes(t, rec.Body.String())
	want := []string{"text_delta", "text_delta", "done"}
	if len(types) != len(want) {
		t.Fatalf("event types = %v, want %v", types, want)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Errorf("event[%d] = %q, want %q", i, types[i], want[i])
		}
	}
}

func TestHandleCreateAIChatMessage_StreamsToolCallProposed(t *testing.T) {
	engine := &fakeAIEngine{
		configured: true,
		runTurn: func(_ context.Context, _, _ string, sink ai.Sink) error {
			sink.ToolCallProposed("conf_1", "tool_1", "deploy_app", json.RawMessage(`{"name":"web"}`))
			return nil
		},
	}
	rt, db := newTestRouterWithAIEngine(t, engine)
	cookie := loginTestSession(t, rt, db)
	_, sessionID := newAIChatSession(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions/"+sessionID+"/messages", `{"content":"deploy web"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	lines := sseDataLines(rec.Body.String())
	if len(lines) != 2 {
		t.Fatalf("lines = %v, want [tool_call_proposed, done]", lines)
	}
	var proposed struct {
		Type           string `json:"type"`
		ConfirmationID string `json:"confirmation_id"`
		ToolUseID      string `json:"tool_use_id"`
		Name           string `json:"name"`
		ReadOnly       bool   `json:"read_only"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &proposed); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if proposed.Type != "tool_call_proposed" || proposed.ConfirmationID != "conf_1" || proposed.Name != "deploy_app" || proposed.ReadOnly {
		t.Errorf("proposed = %+v, want a pending, non-read-only deploy_app proposal", proposed)
	}
}

func TestHandleCreateAIChatMessage_EngineErrorStillSendsDone(t *testing.T) {
	engine := &fakeAIEngine{
		configured: true,
		runTurn: func(context.Context, string, string, ai.Sink) error {
			return context.DeadlineExceeded
		},
	}
	rt, db := newTestRouterWithAIEngine(t, engine)
	cookie := loginTestSession(t, rt, db)
	_, sessionID := newAIChatSession(t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions/"+sessionID+"/messages", `{"content":"hi"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (headers already sent by the time an engine error happens)", rec.Code, http.StatusOK)
	}
	types := sseEventTypes(t, rec.Body.String())
	if len(types) != 1 || types[0] != "done" {
		t.Errorf("event types = %v, want a lone done event even on engine error", types)
	}
}

func TestHandleResolveAIChatConfirmation_NotFound(t *testing.T) {
	rt, db := newTestRouterWithAIEngine(t, &fakeAIEngine{configured: true})
	cookie := loginTestSession(t, rt, db)
	_, sessionID := newAIChatSession(t, db)

	rec := httptest.NewRecorder()
	target := "/api/v1/ai/sessions/" + sessionID + "/confirmations/does-not-exist"
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, target, `{"approve":true}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestHandleResolveAIChatConfirmation_WrongSession(t *testing.T) {
	rt, db := newTestRouterWithAIEngine(t, &fakeAIEngine{configured: true})
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	sessionA := mustCreateAIChatSession(ctx, t, db)
	sessionB := mustCreateAIChatSession(ctx, t, db)
	confID := seedPendingConfirmation(ctx, t, db, sessionA, store.AIChatConfirmationPending)

	rec := httptest.NewRecorder()
	target := "/api/v1/ai/sessions/" + sessionB + "/confirmations/" + confID
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, target, `{"approve":true}`))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d: a confirmation must not resolve under the wrong session", rec.Code, http.StatusNotFound)
	}
}

func TestHandleResolveAIChatConfirmation_AlreadyResolved(t *testing.T) {
	rt, db := newTestRouterWithAIEngine(t, &fakeAIEngine{configured: true})
	cookie := loginTestSession(t, rt, db)
	ctx, sessionID := newAIChatSession(t, db)
	confID := seedPendingConfirmation(ctx, t, db, sessionID, store.AIChatConfirmationApproved)

	rec := httptest.NewRecorder()
	target := "/api/v1/ai/sessions/" + sessionID + "/confirmations/" + confID
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, target, `{"approve":true}`))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestHandleResolveAIChatConfirmation_Success(t *testing.T) {
	var gotApprove bool
	engine := &fakeAIEngine{
		configured: true,
		resolve: func(_ context.Context, _, _ string, approve bool, sink ai.Sink) error {
			gotApprove = approve
			sink.ToolResult("t1", "restart_app", json.RawMessage(`{"status":"ok"}`), false)
			sink.TextDelta("restarted")
			return nil
		},
	}
	rt, db := newTestRouterWithAIEngine(t, engine)
	cookie := loginTestSession(t, rt, db)
	ctx, sessionID := newAIChatSession(t, db)
	confID := seedPendingConfirmation(ctx, t, db, sessionID, store.AIChatConfirmationPending)

	rec := httptest.NewRecorder()
	target := "/api/v1/ai/sessions/" + sessionID + "/confirmations/" + confID
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, target, `{"approve":true}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !gotApprove {
		t.Error("engine received approve=false, want true")
	}
	types := sseEventTypes(t, rec.Body.String())
	want := []string{"tool_result", "text_delta", "done"}
	if len(types) != len(want) {
		t.Fatalf("event types = %v, want %v", types, want)
	}
}

func TestAIChatSessionRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)

	routes := []routeCase{
		{http.MethodPost, "/api/v1/ai/sessions"},
		{http.MethodGet, "/api/v1/ai/sessions/x"},
		{http.MethodPost, "/api/v1/ai/sessions/x/messages"},
		{http.MethodPost, "/api/v1/ai/sessions/x/confirmations/y"},
	}
	for _, r := range routes {
		t.Run(r.method+" "+r.target, func(t *testing.T) {
			req := httptest.NewRequest(r.method, r.target, nil)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestAIChatSessionRoutes_PlainWriteToken_Forbidden(t *testing.T) {
	rt, db := newTestRouterWithAIEngine(t, &fakeAIEngine{configured: true})
	ctx := context.Background()

	const plaintext = "write-scoped-token-ai" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(ctx, store.APIToken{
		ID: "tok_write_ai_chat", Name: "writer", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/sessions", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d: a plain write token must not reach ai chat routes (root-only)", rec.Code, http.StatusForbidden)
	}
}
