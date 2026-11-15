package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/eventhook"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
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
	rt.sweepRotationGrace(ctx, time.Now().Add(48*time.Hour))
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
