package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

func TestSetupTokenGuessing_IsRateLimitedPerIP(t *testing.T) {
	t.Parallel()
	rt, _, token := newSetupTestRouter(t)
	post := func(setup, ip string) int {
		body := `{"username":"admin","password":"a-good-password","setup_token":"` + setup + `"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		req.RemoteAddr = ip + ":4000"
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}

	for i := 0; i < loginGraceFailures+1; i++ {
		if got := post("wrong", "198.51.100.7"); got != http.StatusForbidden {
			t.Fatalf("guess %d = %d, want 403", i, got)
		}
	}
	if got := post(token, "198.51.100.7"); got != http.StatusTooManyRequests {
		t.Errorf("correct token from a locked IP = %d, want 429", got)
	}
	if got := post(token, "198.51.100.8"); got != http.StatusCreated {
		t.Errorf("correct token from a different IP = %d, want 201", got)
	}
}

func TestWakeTokenGuessing_LocksOutTheGuesser(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	rt.wakeToken = "tok"
	call := func(token, ip string) int {
		req := httptest.NewRequest(http.MethodPost, ingress.WakePath, nil)
		req.RemoteAddr = ip + ":4000"
		req.Header.Set(ingress.WakeTokenHeader, token)
		req.Header.Set(ingress.WakeAppHeader, "missing")
		rec := httptest.NewRecorder()
		rt.handleWakeHook(rec, req)
		return rec.Code
	}

	for i := 0; i < loginGraceFailures+1; i++ {
		if got := call("guess", "198.51.100.7"); got != http.StatusNotFound {
			t.Fatalf("wrong token = %d, want 404", got)
		}
	}
	if got := call("tok", "198.51.100.7"); got != http.StatusNotFound {
		t.Errorf("correct token from a locked IP = %d, want 404 (still opaque)", got)
	}
	if got := call("tok", "198.51.100.8"); got == http.StatusNotFound {
		t.Errorf("correct token from another IP = 404, want it to reach the handler")
	}
}

func TestDeviceTokenPoll_IsRateLimitedPerIP(t *testing.T) {
	t.Parallel()
	rt, _ := newTestRouter(t)
	rt.devicePollLimit = newAPIRateLimiter(3)
	poll := func(ip string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/token", strings.NewReader(`{"device_code":"nope"}`))
		req.RemoteAddr = ip + ":4000"
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	for i := 0; i < 3; i++ {
		if got := poll("198.51.100.7"); got == http.StatusTooManyRequests {
			t.Fatalf("poll %d limited too early", i)
		}
	}
	if got := poll("198.51.100.7"); got != http.StatusTooManyRequests {
		t.Errorf("4th poll = %d, want 429", got)
	}
	if got := poll("198.51.100.8"); got == http.StatusTooManyRequests {
		t.Error("a different IP must have its own budget")
	}
}

func TestLoginLimiter_PrunesIdleKeysAtCap(t *testing.T) {
	t.Parallel()
	l := newLoginLimiter()
	old := time.Now().Add(-2 * loginLimiterIdleAfter)
	for i := 0; i < loginLimiterPruneAt; i++ {
		l.attempts[string(rune('a'+i%26))+strings.Repeat("x", i)] = &loginAttemptState{failures: 1, lastFailure: old}
	}
	locked := &loginAttemptState{failures: 9, lastFailure: old, lockedUntil: time.Now().Add(time.Minute)}
	l.attempts["locked"] = locked

	l.recordFailure("fresh")
	if len(l.attempts) > 3 {
		t.Errorf("map holds %d entries after a prune sweep, want only locked + fresh", len(l.attempts))
	}
	if l.attempts["locked"] != locked {
		t.Error("an actively locked key must survive pruning")
	}
}
