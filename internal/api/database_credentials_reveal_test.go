package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRevealDatabaseCredentialsNeedsStepUp(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	rt.secrets = &fakeSecretSetter{}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/credentials/reveal", `{}`))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("reveal without step-up = %d, want 403", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "\"password\"") {
		t.Errorf("body carries a password field: %s", rec.Body.String())
	}
}

func TestDBRevealLimiterIsPerUser(t *testing.T) {
	t.Setenv(envDBRevealPerMin, "2")
	l := newDBRevealLimiter()
	for i := 0; i < 2; i++ {
		if ok, _ := l.allow("u1"); !ok {
			t.Fatalf("attempt %d blocked early", i)
		}
	}
	ok, retry := l.allow("u1")
	if ok || retry <= 0 || retry > time.Minute {
		t.Fatalf("third attempt allowed=%v retry=%v, want blocked", ok, retry)
	}
	if ok, _ := l.allow("u2"); !ok {
		t.Fatal("another user was throttled")
	}
}
