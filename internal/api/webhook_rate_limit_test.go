package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// newTestRouterWithGitSourceSecretsAndWebhookRateLimit is
// newTestRouterWithGitSourceSecrets (git_sources_test.go) plus
// WithWebhookRateLimit, the same "opt into one extra option" shape
// newTestRouterWithAPIRateLimit (api_rate_limit_test.go) already
// establishes for the general per-actor limiter.
func newTestRouterWithGitSourceSecretsAndWebhookRateLimit(t *testing.T, secrets GitSourceSecrets, perMinute int) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithGitSourceSecrets(secrets), WithWebhookRateLimit(perMinute))
	return rt, db
}

// postWrongSecretWebhook sends one deliberately-wrong-signature push to
// app "web"'s webhook route with remoteAddr as the client address: cheap
// to repeat (never reaches the fetch/deploy path even when allowed
// through) and, more importantly, the exact shape this feature exists to
// throttle before it burns a DB lookup, secret resolution, and a
// saveWebhookDelivery write on every attempt.
func postWrongSecretWebhook(t *testing.T, rt *Router, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/web", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("wrong-secret"), body))
	req.RemoteAddr = remoteAddr
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, req)
	return rec
}

// setupWebhookRateLimitTest is the fixture every scenario below starts
// from: a router with WithWebhookRateLimit(perMinute) applied, app "web"
// seeded, and its git source connected, before the test exercises its
// own rate-limit-specific behavior.
func setupWebhookRateLimitTest(t *testing.T, perMinute int) (*Router, *store.DB, *http.Cookie) {
	t.Helper()
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecretsAndWebhookRateLimit(t, secrets, perMinute)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)
	return rt, db, cookie
}

func TestHandleGitPushWebhook_RateLimit_BlocksAfterBudgetExhausted(t *testing.T) {
	rt, _, _ := setupWebhookRateLimitTest(t, 2)

	for i := 0; i < 2; i++ {
		rec := postWrongSecretWebhook(t, rt, "203.0.113.10:1234")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want %d (within budget, signature still wrong)", i, rec.Code, http.StatusUnauthorized)
		}
	}

	rec := postWrongSecretWebhook(t, rt, "203.0.113.10:1234")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third attempt: status = %d, want %d, body = %s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
	ra := rec.Header().Get("Retry-After")
	if ra == "" {
		t.Fatal("missing Retry-After header on a 429 response")
	}
	if n, err := strconv.Atoi(ra); err != nil || n < 1 {
		t.Errorf("Retry-After = %q, want a positive integer", ra)
	}
	if !strings.Contains(rec.Body.String(), "rate limit") {
		t.Errorf("body = %s, want it to mention the rate limit", rec.Body.String())
	}
}

// TestHandleGitPushWebhook_RateLimit_SkipsExpensivePath is the real
// point of gating this route early: a rejected request must never reach
// the DB lookup, secret resolution, or saveWebhookDelivery write the
// unthrottled request above still triggers. Asserted here by checking
// that no delivery record was persisted for the rejected request, since
// saveWebhookDelivery is the last thing handleGitPushWebhook does before
// writing any response (success or failure) once past the rate-limit
// check.
func TestHandleGitPushWebhook_RateLimit_SkipsExpensivePath(t *testing.T) {
	rt, db, _ := setupWebhookRateLimitTest(t, 1)

	if rec := postWrongSecretWebhook(t, rt, "203.0.113.20:1234"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("first attempt: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	rec := postWrongSecretWebhook(t, rt, "203.0.113.20:1234")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("second attempt: status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}

	deliveries, err := db.ListWebhookDeliveries(context.Background(), "web", 10, nil)
	if err != nil {
		t.Fatalf("ListWebhookDeliveries() error = %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("len(deliveries) = %d, want 1 (only the first, non-rate-limited attempt should ever reach saveWebhookDelivery)", len(deliveries))
	}
}

func TestHandleGitPushWebhook_RateLimit_KeyedPerAppName(t *testing.T) {
	rt, db, cookie := setupWebhookRateLimitTest(t, 1)
	seedApp(t, db, "other")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/other/git-source", `{"repo_url":"https://github.com/org/other.git","branch":"main"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("connect other's git source: status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if rec := postWrongSecretWebhook(t, rt, "203.0.113.30:1234"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("web attempt: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := postWrongSecretWebhook(t, rt, "203.0.113.30:1234"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("web second attempt: status = %d, want %d (this app's own budget is exhausted)", rec.Code, http.StatusTooManyRequests)
	}

	body := pushBody("refs/heads/main")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/github/other", strings.NewReader(string(body)))
	req.Header.Set("X-Hub-Signature-256", sign([]byte("wrong-secret"), body))
	req.RemoteAddr = "203.0.113.30:1234"
	otherRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(otherRec, req)
	if otherRec.Code != http.StatusUnauthorized {
		t.Fatalf("other app, same source IP: status = %d, want %d (a different app name must have its own budget)", otherRec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGitPushWebhook_RateLimit_KeyedPerIP(t *testing.T) {
	rt, _, _ := setupWebhookRateLimitTest(t, 1)

	if rec := postWrongSecretWebhook(t, rt, "203.0.113.40:1234"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("first IP: status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if rec := postWrongSecretWebhook(t, rt, "203.0.113.40:1234"); rec.Code != http.StatusTooManyRequests {
		t.Fatalf("first IP, second attempt: status = %d, want %d", rec.Code, http.StatusTooManyRequests)
	}
	if rec := postWrongSecretWebhook(t, rt, "203.0.113.41:5678"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("second IP: status = %d, want %d (a different client IP must have its own budget)", rec.Code, http.StatusUnauthorized)
	}
}

func TestHandleGitPushWebhook_RateLimit_DisabledByDefault(t *testing.T) {
	// newTestRouterWithGitSourceSecrets never applies WithWebhookRateLimit,
	// the same "unset means unthrottled" shape
	// TestRequireAbility_RateLimit_DisabledByDefault (api_rate_limit_test.go)
	// already establishes for the general per-actor limiter.
	secrets := newFakeGitSourceSecrets()
	rt, db := newTestRouterWithGitSourceSecrets(t, secrets)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	connectGitSource(t, rt, cookie, `{"repo_url":"https://github.com/org/web.git","branch":"main"}`)

	for i := 0; i < 100; i++ {
		rec := postWrongSecretWebhook(t, rt, "203.0.113.50:1234")
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want %d (no webhook rate limit configured)", i, rec.Code, http.StatusUnauthorized)
		}
	}
}
