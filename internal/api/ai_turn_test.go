package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ai"
	"github.com/GLINCKER/levelrail/internal/store"
)

func serveAI(rt *Router, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func createSessionAs(t *testing.T, rt *Router, cookie *http.Cookie) string {
	t.Helper()
	rec := serveAI(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions", ""))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create session = %d: %s", rec.Code, rec.Body.String())
	}
	var got aiChatSessionCreatedResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got.ID
}

func TestAIChatSessions_OtherRootPrincipalsGetNotFound(t *testing.T) {
	rt, db := newTestRouterWithAIEngine(t, &fakeAIEngine{configured: true})
	ctx := context.Background()
	alice := storeUserWithAbilitiesForTest(t, db, "alice@example.com", []string{AbilityRoot})
	bob := storeUserWithAbilitiesForTest(t, db, "bob@example.com", []string{AbilityRoot})
	aliceCookie := sessionCookieForTest(t, rt, alice.ID)
	bobCookie := sessionCookieForTest(t, rt, bob.ID)
	rootToken, _, err := MintAPIToken(ctx, db, "ops", []string{AbilityRoot}, nil)
	if err != nil {
		t.Fatal(err)
	}

	sessionID := createSessionAs(t, rt, aliceCookie)
	confID := seedPendingConfirmation(ctx, t, db, sessionID, store.AIChatConfirmationPending)

	others := map[string]func(method, target, body string) *http.Request{
		"other user":        func(m, p, b string) *http.Request { return authedRequest(t, bobCookie, m, p, b) },
		"system root token": func(m, p, b string) *http.Request { return bearerRequest(m, p, b, rootToken) },
	}
	for who, req := range others {
		t.Run(who, func(t *testing.T) {
			list := serveAI(rt, req(http.MethodGet, "/api/v1/ai/sessions", ""))
			if list.Code != http.StatusOK || strings.Contains(list.Body.String(), sessionID) {
				t.Errorf("list = %d %s, want 200 without alice's session", list.Code, list.Body.String())
			}
			for _, c := range []struct{ method, target, body string }{
				{http.MethodGet, "/api/v1/ai/sessions/" + sessionID, ""},
				{http.MethodPost, "/api/v1/ai/sessions/" + sessionID + "/messages", `{"content":"hi"}`},
				{http.MethodPost, "/api/v1/ai/sessions/" + sessionID + "/confirmations/" + confID, `{"approve":true}`},
				{http.MethodDelete, "/api/v1/ai/sessions/" + sessionID, ""},
			} {
				if rec := serveAI(rt, req(c.method, c.target, c.body)); rec.Code != http.StatusNotFound {
					t.Errorf("%s %s = %d, want 404", c.method, c.target, rec.Code)
				}
			}
		})
	}

	if rec := serveAI(rt, authedRequest(t, aliceCookie, http.MethodGet, "/api/v1/ai/sessions/"+sessionID, "")); rec.Code != http.StatusOK {
		t.Errorf("owner GET = %d, want 200", rec.Code)
	}
	list := serveAI(rt, authedRequest(t, aliceCookie, http.MethodGet, "/api/v1/ai/sessions", ""))
	if !strings.Contains(list.Body.String(), sessionID) {
		t.Errorf("owner list = %s, want the session", list.Body.String())
	}
}

func TestAIChatTurn_ToolsRunWithShortLivedTokenCappedToSender(t *testing.T) {
	var cred ai.TurnCredential
	var during map[string]int
	var usedAfterRead []string
	var tokenRec *store.APIToken
	var rt *Router
	engine := &fakeAIEngine{configured: true}
	rt, db := newTestRouterWithAIEngine(t, engine)
	ctx := context.Background()
	for _, name := range []string{"prod-web", "staging-web"} {
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "img:1", Port: 3000}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpdateAIControlSettings(ctx, store.AIControlModeObserve, nil, "test"); err != nil {
		t.Fatal(err)
	}
	alice := storeUserWithAbilitiesForTest(t, db, "alice@example.com", []string{AbilityRoot})
	attachTestPolicy(t, db, "deny-prod-read", "Deny", AbilityRead, "app:prod-web", store.PrincipalTypeUser, alice.ID)
	cookie := sessionCookieForTest(t, rt, alice.ID)
	sessionID := createSessionAs(t, rt, cookie)

	engine.runTurn = func(ctx context.Context, _, _ string, _ ai.Sink) error {
		var ok bool
		if cred, ok = ai.TurnCredentialFrom(ctx); !ok || cred.Token == "" {
			t.Fatal("no turn credential on the engine context")
		}
		var err error
		if tokenRec, err = rt.lookupBearerToken(ctx, cred.Token); err != nil {
			t.Fatalf("turn token lookup: %v", err)
		}
		during = map[string]int{
			"read staging": serveAI(rt, bearerRequest(http.MethodGet, "/api/v1/apps/staging-web", "", cred.Token)).Code,
		}
		usedAfterRead = cred.AbilitiesUsed()
		during["read denied prod"] = serveAI(rt, bearerRequest(http.MethodGet, "/api/v1/apps/prod-web", "", cred.Token)).Code
		during["write in observe"] = serveAI(rt, bearerRequest(http.MethodPost, "/api/v1/apps/staging-web/restart", "", cred.Token)).Code
		return nil
	}

	rec := serveAI(rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/ai/sessions/"+sessionID+"/messages", `{"content":"hi"}`))
	if rec.Code != http.StatusOK {
		t.Fatalf("message = %d: %s", rec.Code, rec.Body.String())
	}
	if cred.ActingAs != "alice@example.com" {
		t.Errorf("acting as = %q, want the sender", cred.ActingAs)
	}
	if tokenRec.OwnerUserID != alice.ID || tokenRec.AgentName != AIAssistantAgentName {
		t.Errorf("turn token owner %q agent %q, want alice and the assistant label", tokenRec.OwnerUserID, tokenRec.AgentName)
	}
	if tokenRec.ExpiresAt == nil || time.Until(*tokenRec.ExpiresAt) > time.Hour {
		t.Errorf("turn token expires %v, want short-lived", tokenRec.ExpiresAt)
	}
	want := map[string]int{"read staging": 200, "read denied prod": 403, "write in observe": 403}
	for k, v := range want {
		if during[k] != v {
			t.Errorf("%s = %d, want %d", k, during[k], v)
		}
	}
	if !slices.Equal(usedAfterRead, []string{AbilityRead}) {
		t.Errorf("abilities used after the read = %v, want [read]", usedAfterRead)
	}
	if code := serveAI(rt, bearerRequest(http.MethodGet, "/api/v1/apps/staging-web", "", cred.Token)).Code; code != http.StatusUnauthorized {
		t.Errorf("turn token after the turn = %d, want 401", code)
	}
	if list := serveAI(rt, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/tokens", "")); list.Code != http.StatusOK || strings.Contains(list.Body.String(), tokenRec.ID) {
		t.Errorf("token list = %d %s, want 200 without the spent turn token", list.Code, list.Body.String())
	}

	var onBehalf string
	if err := db.QueryRowContext(ctx, `SELECT on_behalf_of FROM audit_log WHERE path = ? AND method = 'POST'`, "/api/v1/apps/staging-web/restart").Scan(&onBehalf); err != nil {
		t.Fatalf("audit row for the refused write: %v", err)
	}
	if onBehalf != "user:"+alice.ID {
		t.Errorf("audit on_behalf_of = %q, want user:%s", onBehalf, alice.ID)
	}
}
