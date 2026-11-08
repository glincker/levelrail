package secretprovider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const leakyValue = "leaky-secret-value-9f3a" //nolint:gosec // test fixture, not a credential

// A provider that echoes the token and a value in every failure body must never reach an error.
func TestSecretProviderGuardErrorsCarryNoSecret(t *testing.T) {
	bodies := map[string]struct {
		status int
		body   string
	}{
		"unauthorized": {http.StatusUnauthorized, `{"error":"bad token ` + fakeToken + ` ` + leakyValue + `"}`},
		"forbidden":    {http.StatusForbidden, leakyValue + fakeToken},
		"server error": {http.StatusInternalServerError, leakyValue + fakeToken},
		"bad shape":    {http.StatusOK, `not json ` + leakyValue + fakeToken},
		"too large":    {http.StatusOK, strings.Repeat(leakyValue, 10000)},
	}
	ref, _, err := ParseRef("infisical://inf/billing/API_KEY")
	if err != nil {
		t.Fatal(err)
	}
	for name, tc := range bodies {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := NewResolver().Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tunables())
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), fakeToken) || strings.Contains(err.Error(), leakyValue) {
				t.Fatalf("error leaks a secret: %v", err)
			}
		})
	}
}

func TestSecretProviderGuardOnePasswordErrorsCarryNoSecret(t *testing.T) {
	srv := onePassServer(t, http.StatusInternalServerError, leakyValue+fakeToken)
	defer srv.Close()
	cfg := Config{Name: "vaults", Type: TypeOnePassword, BaseURL: srv.URL, Project: "Prod"}
	ref, _, err := ParseRef("op://vaults/Stripe/api_key")
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewResolver().Fetch(context.Background(), cfg, fakeToken, ref, tunables())
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), fakeToken) || strings.Contains(err.Error(), leakyValue) {
		t.Fatalf("error leaks a secret: %v", err)
	}
}

func TestSecretProviderGuardRefStringHidesNothingSecret(t *testing.T) {
	ref, ok, err := ParseRef("infisical://inf/billing/API_KEY")
	if err != nil || !ok {
		t.Fatalf("parse: %v %v", ok, err)
	}
	if strings.Contains(ref.String(), fakeToken) {
		t.Fatal("ref string carries a token")
	}
}

func TestSecretProviderRateLimitBucket(t *testing.T) {
	l := newLimiter()
	now := time.Unix(1000, 0)
	for i := 0; i < 2; i++ {
		if d := l.reserve("a", 2, now); d != 0 {
			t.Fatalf("burst call %d delayed %s", i, d)
		}
	}
	if d := l.reserve("a", 2, now); d <= 0 {
		t.Fatal("third call in the same instant must wait")
	}
	if d := l.reserve("b", 2, now); d != 0 {
		t.Fatal("another provider has its own budget")
	}
	if d := l.reserve("c", 0, now); d != 0 {
		t.Fatal("0 turns the cap off")
	}
}

func TestSecretProviderRateLimitFailsClosed(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"secret":{"secretValue":"v"}}`))
	}))
	defer srv.Close()
	r := NewResolver()
	fixed := time.Unix(2000, 0)
	r.now = func() time.Time { return fixed }
	tun := tunables()
	tun.CacheTTL = 0
	tun.MaxRPS = 1
	tun.Timeout = 10 * time.Millisecond
	ref, _, _ := ParseRef("infisical://inf/billing/API_KEY")
	if _, err := r.Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tun); err != nil {
		t.Fatalf("first read: %v", err)
	}
	_, err := r.Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tun)
	if !errors.Is(err, ErrRateLimited) {
		t.Fatalf("second read = %v, want ErrRateLimited", err)
	}
	if calls != 1 {
		t.Fatalf("provider saw %d calls, want 1", calls)
	}
}
