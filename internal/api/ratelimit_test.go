package api

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestLoginLimiter_AllowsWithinGrace(t *testing.T) {
	l := newLoginLimiter()
	key := "1.2.3.4|admin"

	for i := 0; i < loginGraceFailures; i++ {
		if ok, _ := l.allow(key); !ok {
			t.Fatalf("attempt %d: allow() = false, want true (still within grace)", i)
		}
		l.recordFailure(key)
	}
}

func TestLoginLimiter_LocksOutAfterGrace(t *testing.T) {
	l := newLoginLimiter()
	key := "1.2.3.4|admin"

	for i := 0; i < loginGraceFailures+1; i++ {
		l.recordFailure(key)
	}

	ok, retryAfter := l.allow(key)
	if ok {
		t.Fatal("allow() = true after exceeding grace failures, want false")
	}
	if retryAfter <= 0 {
		t.Errorf("retryAfter = %v, want a positive duration", retryAfter)
	}
}

func TestLoginLimiter_BackoffGrows(t *testing.T) {
	l := newLoginLimiter()
	key := "1.2.3.4|admin"

	var previous time.Duration
	for i := 0; i < 5; i++ {
		l.recordFailure(key)
		_, retryAfter := l.allow(key)
		if retryAfter > 0 && retryAfter < previous {
			t.Errorf("failure %d: retryAfter = %v, want >= previous %v (backoff must not shrink)", i, retryAfter, previous)
		}
		if retryAfter > 0 {
			previous = retryAfter
		}
	}
	if previous == 0 {
		t.Fatal("backoff never became positive across 5 failures")
	}
}

func TestLoginLimiter_BackoffCapped(t *testing.T) {
	l := newLoginLimiter()
	key := "1.2.3.4|admin"

	// Far more failures than needed to reach loginMaxBackoff; also
	// exercises the maxBackoffShift guard against Duration overflow.
	for i := 0; i < 200; i++ {
		l.recordFailure(key)
	}

	_, retryAfter := l.allow(key)
	if retryAfter <= 0 {
		t.Fatal("retryAfter <= 0 after 200 failures, want a positive, bounded duration")
	}
	if retryAfter > loginMaxBackoff {
		t.Errorf("retryAfter = %v, want <= loginMaxBackoff (%v)", retryAfter, loginMaxBackoff)
	}
}

func TestLoginLimiter_SuccessClearsHistory(t *testing.T) {
	l := newLoginLimiter()
	key := "1.2.3.4|admin"

	for i := 0; i < loginGraceFailures+2; i++ {
		l.recordFailure(key)
	}
	if ok, _ := l.allow(key); ok {
		t.Fatal("allow() = true before recordSuccess, want false (should still be locked out)")
	}

	l.recordSuccess(key)
	if ok, _ := l.allow(key); !ok {
		t.Fatal("allow() = false after recordSuccess, want true (clean slate)")
	}
}

func TestLoginLimiter_KeysAreIndependent(t *testing.T) {
	l := newLoginLimiter()
	keyA := "1.2.3.4|admin"
	keyB := "5.6.7.8|admin"

	for i := 0; i < loginGraceFailures+2; i++ {
		l.recordFailure(keyA)
	}

	if ok, _ := l.allow(keyA); ok {
		t.Error("keyA: allow() = true, want false (locked out)")
	}
	if ok, _ := l.allow(keyB); !ok {
		t.Error("keyB: allow() = false, want true: a different IP/username pair must not share keyA's lockout")
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		remoteAddr string
		want       string
	}{
		{remoteAddr: "192.0.2.1:1234", want: "192.0.2.1"},
		{remoteAddr: "[::1]:5678", want: "::1"},
		{remoteAddr: "no-port-at-all", want: "no-port-at-all"},
	}
	for _, tt := range tests {
		t.Run(tt.remoteAddr, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if got := clientIP(req); got != tt.want {
				t.Errorf("clientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLoginLimiterKey_CombinesIPAndUsername(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.0.2.1:1234"

	got := loginLimiterKey(req, "admin")
	want := "192.0.2.1|admin"
	if got != want {
		t.Errorf("loginLimiterKey() = %q, want %q", got, want)
	}
}
