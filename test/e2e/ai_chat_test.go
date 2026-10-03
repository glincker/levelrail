// TestAIChatSessionLifecycle_Live is this package's live proof for the
// AI assistant chat feature: a session's full create/message/list/
// delete lifecycle over a real *api.Router and real HTTP, with a real
// ai.Engine (fake Provider/ToolExecutor, no real LLM call), plus the
// flag-off 404. internal/api/ai_chat_test.go covers each handler in
// isolation already; this proves the wiring between them.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	e2eAIChatAdminUsername = "e2e-ai-chat-admin"
	e2eAIChatAdminPassword = "e2e-ai-chat-correct-horse" //nolint:gosec // test fixture credential, not a real secret
)

// fakeAIChatProvider replays a fixed script of turns, one per Complete
// call, mirroring internal/ai/engine_test.go's own fakeProvider (package
// ai, not importable here, so this is a minimal independent copy against
// the exported ai.Provider interface only).
type fakeAIChatProvider struct {
	turns []ai.TurnResult
	i     int
}

func (f *fakeAIChatProvider) Complete(_ context.Context, _ ai.ChatRequest, onText ai.TextDeltaFunc) (ai.TurnResult, error) {
	turn := f.turns[f.i]
	f.i++
	if turn.Text != "" {
		onText(turn.Text)
	}
	return turn, nil
}

// fakeAIChatTools is a scripted ai.ToolExecutor exposing one read-only
// tool, so the engine's auto-execute path (no human confirmation needed)
// runs for real without calling the platform's own MCP tool surface.
type fakeAIChatTools struct{}

func (fakeAIChatTools) ListTools(context.Context) ([]ai.ToolSpec, error) {
	return []ai.ToolSpec{{
		Name:        "list_apps",
		Description: "list every app",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Traits:      ai.ToolTraits{Known: true, ReadOnly: true},
	}}, nil
}

func (fakeAIChatTools) Call(context.Context, string, json.RawMessage) (string, bool, error) {
	return `[{"name":"web"}]`, false, nil
}

// aiSessionCreatedBody and aiSessionDetailBody mirror internal/api's own
// unexported aiChatSessionCreatedResource/aiChatSessionResource JSON
// shapes, the same cross-package-boundary decoding authResponse already
// establishes in auth_lifecycle_test.go.
type aiSessionCreatedBody struct {
	ID string `json:"id"`
}

type aiSessionMessageBody struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type aiSessionDetailBody struct {
	ID       string                 `json:"id"`
	Messages []aiSessionMessageBody `json:"messages"`
}

type aiSessionSummaryBody struct {
	ID string `json:"id"`
}

func TestAIChatSessionLifecycle_Live(t *testing.T) {
	db := openLiveStore(t)
	ctx := context.Background()

	masterKey, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("GenerateMasterKey() error = %v", err)
	}
	manager := secrets.NewManager(db, masterKey)

	if err := db.UpdateAIAssistantSettings(ctx, store.AIAssistantSettings{
		Provider: store.AIProviderAnthropic, Model: "test-model",
	}); err != nil {
		t.Fatalf("UpdateAIAssistantSettings() error = %v", err)
	}
	if err := manager.SetValue(ctx, store.AIAssistantSecretsKey(), ai.SecretsAPIKeyEnvKey, "sk-test-fake-key"); err != nil {
		t.Fatalf("SetValue() error = %v", err)
	}

	// Scripted turn loop: the model first asks to run the read-only
	// list_apps tool (auto-executed, no confirmation needed), then ends
	// the turn with a plain reply, proving the tool round trip and the
	// final assistant message both travel correctly end to end.
	fp := &fakeAIChatProvider{turns: []ai.TurnResult{
		{ToolCalls: []ai.ToolUseCall{{ID: "call_1", Name: "list_apps", Input: json.RawMessage(`{}`)}}, StopReason: ai.StopReasonToolUse},
		{Text: "You have 1 app: web.", StopReason: ai.StopReasonEndTurn},
	}}
	engine := ai.NewEngine(db, manager, fakeAIChatTools{}, func(string, string) (ai.Provider, error) { return fp, nil }, "Test Platform")

	logger := discardTestLogger()
	b := &brand.Brand{Name: "Test Platform", BinaryName: "testplatform"}
	router := api.NewRouter(logger, b, db, api.WithAIEngine(engine), api.WithAIAssistantSecrets(manager))
	ts := newE2ETestServer(t, router)

	if err := api.BootstrapAdmin(ctx, db, e2eAIChatAdminUsername, e2eAIChatAdminPassword); err != nil {
		t.Fatalf("BootstrapAdmin() error = %v", err)
	}
	client := loginE2EClient(t, ts.URL, e2eAIChatAdminUsername, e2eAIChatAdminPassword)

	// Step 1: the experimental flag starts off in this test binary (no
	// APP_EXPERIMENTAL set), so the route must 404 cleanly, the same
	// gated-off shape internal/api/experimental_gate_test.go proves at
	// the middleware level, now proven over a real HTTP round trip.
	experimental.Set()
	t.Cleanup(experimental.Reset)
	status, body := requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/ai/sessions", "")
	if status != http.StatusNotFound {
		t.Fatalf("create session with flag off: status = %d, want %d, body = %s", status, http.StatusNotFound, body)
	}

	// Step 2: flip the flag on and drive the real lifecycle.
	experimental.Set(experimental.AIChat)

	status, body = requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/ai/sessions", "")
	if status != http.StatusCreated {
		t.Fatalf("create session: status = %d, want %d, body = %s", status, http.StatusCreated, body)
	}
	var created aiSessionCreatedBody
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatalf("decode create response: %v, body = %s", err, body)
	}
	if created.ID == "" {
		t.Fatal("created session id is empty")
	}

	// Step 3: send a message and read the real SSE stream to its end.
	status, body = requestJSON(t, client, http.MethodPost, ts.URL+"/api/v1/ai/sessions/"+created.ID+"/messages", `{"content":"what apps do I have?"}`)
	if status != http.StatusOK {
		t.Fatalf("send message: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	events := parseSSEEvents(t, string(body))
	assertHasSSEEventType(t, events, "tool_result")
	assertHasSSEEventType(t, events, "done")

	// Step 4: GET the session back and confirm the real stored
	// transcript: the user message, the tool round trip's silence, and
	// the model's final plain-text reply.
	status, body = requestJSON(t, client, http.MethodGet, ts.URL+"/api/v1/ai/sessions/"+created.ID, "")
	if status != http.StatusOK {
		t.Fatalf("get session: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	var detail aiSessionDetailBody
	if err := json.Unmarshal(body, &detail); err != nil {
		t.Fatalf("decode session detail: %v, body = %s", err, body)
	}
	if detail.ID != created.ID {
		t.Errorf("detail.ID = %q, want %q", detail.ID, created.ID)
	}
	var sawUser, sawAssistantReply bool
	for _, m := range detail.Messages {
		if m.Role == store.AIChatRoleUser && m.Content == "what apps do I have?" {
			sawUser = true
		}
		if m.Role == store.AIChatRoleAssistant && m.Content == "You have 1 app: web." {
			sawAssistantReply = true
		}
	}
	if !sawUser {
		t.Errorf("messages = %+v, want the original user message", detail.Messages)
	}
	if !sawAssistantReply {
		t.Errorf("messages = %+v, want the model's final reply", detail.Messages)
	}

	// Step 5: list sessions, the session must appear exactly once.
	status, body = requestJSON(t, client, http.MethodGet, ts.URL+"/api/v1/ai/sessions", "")
	if status != http.StatusOK {
		t.Fatalf("list sessions: status = %d, want %d, body = %s", status, http.StatusOK, body)
	}
	var list []aiSessionSummaryBody
	if err := json.Unmarshal(body, &list); err != nil {
		t.Fatalf("decode session list: %v, body = %s", err, body)
	}
	found := 0
	for _, s := range list {
		if s.ID == created.ID {
			found++
		}
	}
	if found != 1 {
		t.Errorf("session %q appeared %d times in the list, want exactly 1 (list = %+v)", created.ID, found, list)
	}

	// Step 6: delete the session, then prove it's actually gone through
	// the same real HTTP GET, not by reading store state directly.
	status, body = requestJSON(t, client, http.MethodDelete, ts.URL+"/api/v1/ai/sessions/"+created.ID, "")
	if status != http.StatusNoContent {
		t.Fatalf("delete session: status = %d, want %d, body = %s", status, http.StatusNoContent, body)
	}
	status, body = requestJSON(t, client, http.MethodGet, ts.URL+"/api/v1/ai/sessions/"+created.ID, "")
	if status != http.StatusNotFound {
		t.Fatalf("get session after delete: status = %d, want %d, body = %s", status, http.StatusNotFound, body)
	}
}

// sseEvent is one parsed "data: <json>\n\n" line, keyed by its own
// "type" discriminator field, the same framing internal/api/ai_chat.go's
// aiSSESink.write documents.
type sseEvent struct {
	Type string `json:"type"`
}

// parseSSEEvents splits a full SSE response body into its individual
// "data: ..." events, skipping the leading ": connected" comment line
// startAISSE writes to prime the stream.
func parseSSEEvents(t *testing.T, body string) []sseEvent {
	t.Helper()
	var out []sseEvent
	for _, chunk := range strings.Split(body, "\n\n") {
		line := strings.TrimSpace(chunk)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var ev sseEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev); err != nil {
			t.Fatalf("decode sse event %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

func assertHasSSEEventType(t *testing.T, events []sseEvent, want string) {
	t.Helper()
	for _, ev := range events {
		if ev.Type == want {
			return
		}
	}
	t.Errorf("sse events = %+v, want one of type %q", events, want)
}
