package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAPIRateLimiter_AllowsBurstThenBlocks(t *testing.T) {
	l := newAPIRateLimiter(60)
	key := "actor"

	for i := 0; i < 60; i++ {
		if ok, _ := l.allow(key); !ok {
			t.Fatalf("attempt %d: allow() = false, want true (within initial burst)", i)
		}
	}
	ok, retryAfter := l.allow(key)
	if ok {
		t.Fatal("allow() = true after exhausting the burst, want false")
	}
	if retryAfter <= 0 {
		t.Errorf("retryAfter = %v, want a positive duration", retryAfter)
	}
}

func TestAPIRateLimiter_RefillsOverTime(t *testing.T) {
	l := newAPIRateLimiter(60) // 1 token/sec
	key := "actor"

	for i := 0; i < 60; i++ {
		if ok, _ := l.allow(key); !ok {
			t.Fatalf("attempt %d: allow() = false, want true", i)
		}
	}
	if ok, _ := l.allow(key); ok {
		t.Fatal("allow() = true immediately after exhausting the burst, want false")
	}

	// Simulate 2 seconds passing without a real sleep, by back-dating the
	// bucket's own lastRefill: same-package white-box access, the shape
	// ratelimit_test.go's own loginLimiter tests already use on that
	// type's exported allow()/recordFailure() instead.
	l.mu.Lock()
	l.buckets[key].lastRefill = l.buckets[key].lastRefill.Add(-2 * time.Second)
	l.mu.Unlock()

	if ok, _ := l.allow(key); !ok {
		t.Fatal("allow() = false after a simulated refill window, want true")
	}
}

func TestAPIRateLimiter_KeysAreIndependent(t *testing.T) {
	l := newAPIRateLimiter(1)

	if ok, _ := l.allow("actor-a"); !ok {
		t.Fatal("actor-a's first call should be allowed")
	}
	if ok, _ := l.allow("actor-a"); ok {
		t.Fatal("actor-a's second call should be blocked")
	}
	if ok, _ := l.allow("actor-b"); !ok {
		t.Fatal("actor-b's first call should be allowed, unaffected by actor-a's budget")
	}
}

func TestAPIRateLimiter_DisabledWhenRateIsZeroOrLess(t *testing.T) {
	for _, rate := range []int{0, -1} {
		l := newAPIRateLimiter(rate)
		for i := 0; i < 500; i++ {
			if ok, _ := l.allow("actor"); !ok {
				t.Fatalf("rate=%d, attempt %d: allow() = false, want true (a non-positive rate disables the limit)", rate, i)
			}
		}
	}
}

func TestAPIRateLimitTier(t *testing.T) {
	tests := []struct {
		ability string
		want    string
	}{
		{AbilityRead, "read"},
		{AbilityReadSensitive, "write"},
		{AbilityWrite, "write"},
		{AbilityWriteSensitive, "write"},
		{AbilityDeploy, "write"},
		{AbilityRoot, "write"},
	}
	for _, tt := range tests {
		if got := apiRateLimitTier(tt.ability); got != tt.want {
			t.Errorf("apiRateLimitTier(%q) = %q, want %q", tt.ability, got, tt.want)
		}
	}
}

func TestAPIRateLimit_ReadAndWriteBudgetsAreIndependent(t *testing.T) {
	l := newAPIRateLimit(1, 1)
	key := "actor"

	if ok, _ := l.allow(AbilityRead, key); !ok {
		t.Fatal("first read call should be allowed")
	}
	if ok, _ := l.allow(AbilityRead, key); ok {
		t.Fatal("second read call should be blocked (read budget is 1)")
	}
	if ok, _ := l.allow(AbilityWrite, key); !ok {
		t.Fatal("first write call should be allowed even though the read budget for the same key is exhausted")
	}
}

// newTestRouterWithAPIRateLimit is newTestRouter plus WithAPIRateLimit,
// the same "opt into one extra option" shape every other
// newTestRouterWith* helper in this package already uses. Every other
// test in this package uses plain newTestRouter, which never applies
// this option, so the rate limiter stays off (rt.apiRateLimit stays
// nil) for the rest of the suite: see
// TestRequireAbility_RateLimit_DisabledByDefault below for an explicit
// regression guard on that.
func newTestRouterWithAPIRateLimit(t *testing.T, readPerMinute, writePerMinute int) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithAPIRateLimit(readPerMinute, writePerMinute))
	return rt, db
}

func TestRequireAbility_RateLimit_DisabledByDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	for i := 0; i < 250; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
		req.AddCookie(cookie)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("attempt %d: status = %d, want %d (no rate limit configured, per WithAPIRateLimit's own doc comment)", i, rec.Code, http.StatusOK)
		}
	}
}

func TestRequireAbility_RateLimit_TokenWriteBudgetExhausted(t *testing.T) {
	rt, db := newTestRouterWithAPIRateLimit(t, 100, 2)

	const plaintext = "rl-write-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_rl_write", Name: "rl bot", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite, AbilityRead}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	createApp := func(name string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"`+name+`","image":"levelrail/x:1","port":4000}`))
		req.Header.Set("Authorization", "Bearer "+plaintext)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}

	if rec := createApp("rl-app-one"); rec.Code != http.StatusCreated {
		t.Fatalf("first create status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if rec := createApp("rl-app-two"); rec.Code != http.StatusCreated {
		t.Fatalf("second create status = %d, want %d, body = %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	rec := createApp("rl-app-three")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third create status = %d, want %d, body = %s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
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

	// The read-tier budget for the same token is untouched: exhausting
	// writes must never bleed into reads.
	readReq := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	readReq.Header.Set("Authorization", "Bearer "+plaintext)
	readRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("read after write budget exhausted: status = %d, want %d, body = %s", readRec.Code, http.StatusOK, readRec.Body.String())
	}
}

func TestRequireAbility_RateLimit_SessionAndTokenBudgetsAreIndependent(t *testing.T) {
	rt, db := newTestRouterWithAPIRateLimit(t, 100, 1)
	cookie := loginTestSession(t, rt, db)

	const plaintext = "rl-independent-token" //nolint:gosec // fake fixture, not a real credential
	if err := db.SaveAPIToken(context.Background(), store.APIToken{
		ID: "tok_rl_independent", Name: "rl bot 2", TokenHash: hashToken(plaintext), Abilities: []string{AbilityWrite}, CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	tokenReq1 := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"tok-app-1","image":"levelrail/x:1","port":4000}`))
	tokenReq1.Header.Set("Authorization", "Bearer "+plaintext)
	rec1 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec1, tokenReq1)
	if rec1.Code != http.StatusCreated {
		t.Fatalf("token first create status = %d, want %d, body = %s", rec1.Code, http.StatusCreated, rec1.Body.String())
	}

	tokenReq2 := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"tok-app-2","image":"levelrail/x:1","port":4000}`))
	tokenReq2.Header.Set("Authorization", "Bearer "+plaintext)
	rec2 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec2, tokenReq2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("token second create status = %d, want %d, body = %s", rec2.Code, http.StatusTooManyRequests, rec2.Body.String())
	}

	sessionReq := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"session-app","image":"levelrail/x:1","port":4000}`))
	sessionReq.AddCookie(cookie)
	rec3 := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec3, sessionReq)
	if rec3.Code != http.StatusCreated {
		t.Fatalf("session create status = %d, want %d, body = %s (the token's exhausted budget must not affect the session actor)", rec3.Code, http.StatusCreated, rec3.Body.String())
	}
}

func TestRequireAbility_RateLimit_UnauthenticatedByIP(t *testing.T) {
	rt, _ := newTestRouterWithAPIRateLimit(t, 2, 100)

	doGet := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec
	}

	for i := 0; i < 2; i++ {
		rec := doGet()
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want %d (no credentials supplied)", i, rec.Code, http.StatusUnauthorized)
		}
	}

	rec := doGet()
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third unauthenticated attempt: status = %d, want %d, body = %s", rec.Code, http.StatusTooManyRequests, rec.Body.String())
	}
}
