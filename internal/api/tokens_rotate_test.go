package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/eventhook"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

type captureWebhooks struct {
	EventWebhookService
	events []eventhook.Event
}

func (c *captureWebhooks) Emit(e eventhook.Event) { c.events = append(c.events, e) }

// tokenUsable reports whether a bearer secret still authenticates: a revoked row is returned without an error.
func tokenUsable(rt *Router, raw string) bool {
	rec, err := rt.lookupBearerToken(context.Background(), raw)
	return err == nil && rec.RevokedAt == nil
}

func (c *captureWebhooks) tokenEvents() []eventhook.Event {
	var out []eventhook.Event
	for _, e := range c.events {
		if strings.HasPrefix(e.Type, "tokens.") {
			out = append(out, e)
		}
	}
	return out
}

func rotateAs(t *testing.T, rt *Router, cookie *http.Cookie, id string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens/"+id+"/rotate", ""))
	return rec
}

func TestRotateTokenKeepsOldValidDuringGraceThenRevokes(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.gitConnSecrets = newFakeGitConnSecrets()
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	cookie := loginTestSession(t, rt, db)
	old := mintTokenAs(t, rt, cookie, "ci")

	rec := rotateAs(t, rt, cookie, old.ID)
	if rec.Code != http.StatusCreated {
		t.Fatalf("rotate status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out rotateTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Token == "" || out.Token == old.Token || out.Replaces != old.ID || out.Name != "ci" || out.PreviousValidUntil == nil {
		t.Fatalf("unexpected rotate response: %+v", out)
	}
	ctx := context.Background()
	if !tokenUsable(rt, old.Token) {
		t.Fatal("old token must stay valid during grace")
	}
	if err := rt.sweepRotationGrace(ctx, time.Now().Add(48*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if tokenUsable(rt, old.Token) {
		t.Fatal("old token must be revoked after grace")
	}
	if !tokenUsable(rt, out.Token) {
		t.Fatal("new token must work")
	}
}

func TestRotateTokenWithoutStoreRevokesAtOnce(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	old := mintTokenAs(t, rt, cookie, "ci")
	if rec := rotateAs(t, rt, cookie, old.ID); rec.Code != http.StatusCreated {
		t.Fatalf("rotate status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if tokenUsable(rt, old.Token) {
		t.Fatal("old token must be revoked immediately when no grace can be remembered")
	}
}

func TestRotateTokenRefusesSystemAndOthersTokens(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	_, sys, err := MintAPIToken(context.Background(), rt.tokens, "system-thing", []string{AbilityRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if rec := rotateAs(t, rt, cookie, sys.ID); rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("system token rotate status = %d, want 403 or 404", rec.Code)
	}
	alice := storeUserWithAbilitiesForTest(t, db, "alice@example.com", []string{AbilityRead})
	aliceCookie := sessionCookieForTest(t, rt, alice.ID)
	mine := mintTokenAs(t, rt, cookie, "admins")
	if rec := rotateAs(t, rt, aliceCookie, mine.ID); rec.Code != http.StatusNotFound {
		t.Fatalf("other user's token rotate status = %d, want 404", rec.Code)
	}
}

func TestExpiryReminderSentOncePerWindowThenOnExpiry(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	hooks := &captureWebhooks{}
	rt.eventWebhooks = hooks
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"soon","abilities":["read"],"expires_in_days":5}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint status = %d", rec.Code)
	}
	ctx, now := context.Background(), time.Now()
	for range 3 {
		if err := rt.SweepTokenExpiry(ctx, now); err != nil {
			t.Fatal(err)
		}
	}
	if len(hooks.tokenEvents()) != 1 || hooks.tokenEvents()[0].Type != eventTokenExpiring {
		t.Fatalf("events after repeated sweeps = %+v, want one expiring", hooks.tokenEvents())
	}
	if err := rt.SweepTokenExpiry(ctx, now.Add(6*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := rt.SweepTokenExpiry(ctx, now.Add(6*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if len(hooks.tokenEvents()) != 2 || hooks.tokenEvents()[1].Type != eventTokenExpired {
		t.Fatalf("events after expiry = %+v, want a second expired event once", hooks.tokenEvents())
	}
}

func TestReminderLevel(t *testing.T) {
	w := []int{14, 7, 1}
	day := 24 * time.Hour
	for _, c := range []struct {
		left time.Duration
		want int
	}{{30 * day, -1}, {10 * day, 14}, {3 * day, 7}, {day / 2, 1}, {-time.Hour, 0}} {
		if got := reminderLevel(c.left, w); got != c.want {
			t.Errorf("left %s = %d, want %d", c.left, got, c.want)
		}
	}
}

func newGraceRouter(t *testing.T) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	rt, db := newTestRouter(t)
	rt.gitConnSecrets = newFakeGitConnSecrets()
	rt.runtimeSettings = runtimesettings.New(db, func(string) (string, bool) { return "", false })
	return rt, db, loginTestSession(t, rt, db)
}

func mintTokenBody(t *testing.T, rt *Router, cookie *http.Cookie, body string) createTokenResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", body))
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func rotateWithBearer(t *testing.T, rt *Router, raw, id string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/tokens/"+id+"/rotate", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

func decodeRotation(t *testing.T, rec *httptest.ResponseRecorder) rotateTokenResponse {
	t.Helper()
	if rec.Code != http.StatusCreated {
		t.Fatalf("rotate status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var out rotateTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func pendingRevocation(t *testing.T, rt *Router, id string) (int64, bool) {
	t.Helper()
	m, err := rt.tokenStateLoad(context.Background(), tokenStateRotation)
	if err != nil {
		t.Fatal(err)
	}
	at, ok := m[id]
	return at, ok
}

func TestRotateTokenInGraceIsRefusedAndKeepsItsRevocationTime(t *testing.T) {
	rt, _, cookie := newGraceRouter(t)
	old := mintTokenBody(t, rt, cookie, `{"name":"ci","abilities":["write"]}`)
	decodeRotation(t, rotateWithBearer(t, rt, old.Token, old.ID))
	before, ok := pendingRevocation(t, rt, old.ID)
	if !ok {
		t.Fatal("first rotation must record a pending revocation")
	}
	if rec := rotateWithBearer(t, rt, old.Token, old.ID); rec.Code != http.StatusConflict {
		t.Fatalf("self rotation in grace status = %d, want 409", rec.Code)
	}
	if rec := rotateAs(t, rt, cookie, old.ID); rec.Code != http.StatusConflict {
		t.Fatalf("session rotation in grace status = %d, want 409", rec.Code)
	}
	if after, _ := pendingRevocation(t, rt, old.ID); after != before {
		t.Fatalf("pending revocation moved from %d to %d", before, after)
	}
}

func TestRotateTokenRevokedDuringRotationRevokesTheReplacement(t *testing.T) {
	rt, _, cookie := newGraceRouter(t)
	old := mintTokenAs(t, rt, cookie, "ci")
	var replacements []string
	rotationAfterMint = func(id string) {
		if err := rt.libraryRevokeToken(context.Background(), id); err != nil {
			t.Error(err)
		}
	}
	t.Cleanup(func() { rotationAfterMint = nil })
	if rec := rotateAs(t, rt, cookie, old.ID); rec.Code != http.StatusConflict {
		t.Fatalf("rotate status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	toks, err := rt.libraryListTokens(context.Background(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, tok := range toks {
		if tok.Name == "ci" && tok.ID != old.ID {
			replacements = append(replacements, tok.ID)
			if tok.RevokedAt == nil {
				t.Fatalf("replacement %s must be revoked", tok.ID)
			}
		}
	}
	if len(replacements) != 1 {
		t.Fatalf("replacements = %v, want exactly one revoked replacement", replacements)
	}
	if _, ok := pendingRevocation(t, rt, old.ID); ok {
		t.Fatal("a refused rotation must not record a pending revocation")
	}
}

func TestRotateTokenRequiresTheOwnersCurrentAbilities(t *testing.T) {
	rt, db, admin := newGraceRouter(t)
	alice := storeUserWithAbilitiesForTest(t, db, "alice@example.com", []string{AbilityRead, AbilityWrite})
	aliceCookie := sessionCookieForTest(t, rt, alice.ID)
	tok := mintTokenBody(t, rt, aliceCookie, `{"name":"deploy","abilities":["write"]}`)
	if err := db.UpdateUserAbilities(context.Background(), alice.ID, []string{AbilityRead}); err != nil {
		t.Fatal(err)
	}
	rec := rotateAs(t, rt, aliceCookie, tok.ID)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), AbilityWrite) {
		t.Fatalf("rotate after losing write = %d %s, want 403 naming the ability", rec.Code, rec.Body.String())
	}
	if rec := rotateAs(t, rt, admin, tok.ID); rec.Code != http.StatusForbidden {
		t.Fatalf("admin rotate of a token beyond its owner = %d, want 403", rec.Code)
	}
}

func TestRotateTokenSessionOnlyAbilityNeedsTheOwnersSession(t *testing.T) {
	rt, _, cookie := newGraceRouter(t)
	tok := mintTokenBody(t, rt, cookie, `{"name":"reveal","abilities":["write","secrets:reveal"]}`)
	if rec := rotateWithBearer(t, rt, tok.Token, tok.ID); rec.Code != http.StatusForbidden {
		t.Fatalf("self rotation with %s = %d, want 403", AbilitySecretsReveal, rec.Code)
	}
	out := decodeRotation(t, rotateAs(t, rt, cookie, tok.ID))
	if !strings.Contains(strings.Join(out.Abilities, ","), AbilitySecretsReveal) {
		t.Fatalf("owner session rotation abilities = %v", out.Abilities)
	}
}

func TestRotateTokenWithoutExpiryTakesTheDefaultUnderAMaximumLifetime(t *testing.T) {
	rt, _, cookie := newGraceRouter(t)
	tok := mintTokenBody(t, rt, cookie, `{"name":"forever","abilities":["read"],"no_expiry":true}`)
	t.Setenv(envTokenMaxLifetimeDays, "30")
	out := decodeRotation(t, rotateAs(t, rt, cookie, tok.ID))
	limit := time.Now().Add(30*24*time.Hour + time.Minute)
	if out.ExpiresAt == nil || out.ExpiresAt.After(limit) {
		t.Fatalf("replacement expires_at = %v, want at most 30 days out", out.ExpiresAt)
	}
}

func TestRotateTokenRefusesMCPOAuthTokens(t *testing.T) {
	rt, _, cookie := newGraceRouter(t)
	owner, _ := rt.sessions.lookup(cookie.Value)
	_, id, err := rt.mintMCPOAuthToken(context.Background(), owner, "client", []string{AbilityRead}, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	rec := rotateAs(t, rt, cookie, id)
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "MCP") {
		t.Fatalf("MCP OAuth token rotate = %d %s, want 409", rec.Code, rec.Body.String())
	}
}

func TestRotateTokenAuditsTheRealActorIncludingRefusals(t *testing.T) {
	rt, db, cookie := newGraceRouter(t)
	owner, _ := rt.sessions.lookup(cookie.Value)
	bySession := mintTokenAs(t, rt, cookie, "by-session")
	decodeRotation(t, rotateAs(t, rt, cookie, bySession.ID))
	if rec := rotateAs(t, rt, cookie, bySession.ID); rec.Code != http.StatusConflict {
		t.Fatalf("second rotate = %d, want 409", rec.Code)
	}
	self := mintTokenBody(t, rt, cookie, `{"name":"self","abilities":["write"]}`)
	decodeRotation(t, rotateWithBearer(t, rt, self.Token, self.ID))

	entries, err := db.ListAuditEntries(context.Background(), 200, nil, store.AuditEntryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	rows := func(id string) []store.AuditEntry {
		var out []store.AuditEntry
		for _, e := range entries {
			if e.Path == "/api/v1/auth/tokens/"+id+"/rotate" {
				out = append(out, e)
			}
		}
		return out
	}
	sess := rows(bySession.ID)
	if len(sess) != 2 {
		t.Fatalf("session rotation audit rows = %d, want 2 (created and refused)", len(sess))
	}
	codes := map[int]bool{}
	for _, e := range sess {
		if e.ActorType != auditActorSession || e.ActorID != owner {
			t.Fatalf("session rotation actor = %s/%s, want session/%s", e.ActorType, e.ActorID, owner)
		}
		codes[e.StatusCode] = true
	}
	if !codes[http.StatusCreated] || !codes[http.StatusConflict] {
		t.Fatalf("session rotation audit statuses = %v", codes)
	}
	tok := rows(self.ID)
	if len(tok) != 1 || tok[0].ActorType != "token" || tok[0].ActorID != self.ID {
		t.Fatalf("self rotation audit rows = %+v, want one token row for %s", tok, self.ID)
	}
}

func TestRotationGraceSweepReportsUnreadableState(t *testing.T) {
	rt, _, _ := newGraceRouter(t)
	secrets := newFakeGitConnSecrets()
	secrets.existsErr = errFakeSecretNotSet
	rt.gitConnSecrets = secrets
	if err := rt.sweepRotationGrace(context.Background(), time.Now()); err == nil {
		t.Fatal("sweep must report a state it cannot read")
	}
}
