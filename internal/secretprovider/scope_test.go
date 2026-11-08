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

func TestInfisicalPathLimit(t *testing.T) {
	cases := []struct {
		limit, ref, want string
		ok               bool
	}{
		{"", "", "/", true},
		{"", "/billing", "/billing", true},
		{"/app", "", "/app", true},
		{"/app", "/app/db", "/app/db", true},
		{"/app/", "/app", "/app", true},
		{"/app", "/other", "", false},
		{"/app", "/app/../other", "", false},
		{"/app", "/application", "", false},
		{"", "/a/../../b", "/b", true},
	}
	for _, c := range cases {
		got, err := infisicalPath(c.limit, c.ref)
		if (err == nil) != c.ok || got != c.want {
			t.Errorf("infisicalPath(%q, %q) = %q, %v", c.limit, c.ref, got, err)
		}
	}
}

func TestInfisicalRefOutsideProviderPathRefused(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"secret":{"secretValue":"v"}}`))
	}))
	defer srv.Close()
	cfg := selfHosted(srv.URL)
	cfg.Path = "/app"
	ref, _, _ := ParseRef("infisical://inf/app/../admin/ROOT_KEY")
	if _, err := NewResolver().Fetch(context.Background(), cfg, fakeToken, ref, tunables()); err == nil || calls != 0 {
		t.Fatalf("a path outside the provider path must be refused before any call: %v, calls %d", err, calls)
	}
}

func TestSecretProviderRedirectSchemeChangeBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+r.Host+r.URL.Path, http.StatusFound) //nolint:gosec // test server, the scheme change is the point of the test
	}))
	defer srv.Close()
	ref, _, _ := ParseRef("infisical://inf/API_KEY")
	_, err := NewResolver().Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tunables())
	if !errors.Is(err, ErrRedirectBlocked) {
		t.Fatalf("want redirect blocked on a scheme change, got %v", err)
	}
}

func TestSecretProviderEndpointChangeNeedsNewToken(t *testing.T) {
	mem := memSecrets{}
	st := Store{Secrets: mem}
	ctx, now := context.Background(), time.Now()
	cfg := Config{Name: "inf", Type: TypeInfisical, Project: "ws1", Environment: "prod", BaseURL: "https://a.example"}
	if _, err := st.Put(ctx, cfg, "first", time.Hour, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Put(ctx, cfg, "second", time.Hour, now); err != nil {
		t.Fatal(err)
	}
	moved := cfg
	moved.BaseURL = "https://b.example"
	if _, err := st.Put(ctx, moved, "", time.Hour, now); err == nil || !strings.Contains(err.Error(), "new token") {
		t.Fatalf("a base_url change without a token must fail, got %v", err)
	}
	other := cfg
	other.Project = "ws2"
	if _, err := st.Put(ctx, other, "", time.Hour, now); err == nil {
		t.Fatal("a project change without a token must fail")
	}
	saved, err := st.Put(ctx, moved, "third", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	cur, prev, _ := st.Tokens(ctx, saved, now)
	if cur != "third" || prev != "" || !saved.GraceUntil.IsZero() {
		t.Fatalf("the old token must not carry over to a new endpoint: cur %q prev %q grace %v", cur, prev, saved.GraceUntil)
	}
	env := cfg
	env.BaseURL = moved.BaseURL
	env.Environment = "staging"
	if _, err := st.Put(ctx, env, "", time.Hour, now); err != nil {
		t.Fatalf("other edits keep the stored token: %v", err)
	}
}

func TestStoreResolvesLive(t *testing.T) {
	mem := memSecrets{}
	st := Store{Secrets: mem}
	ctx := context.Background()
	if _, err := st.Put(ctx, Config{Name: "onep", Type: TypeOnePassword, BaseURL: "https://op.example"}, "tok", 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		"plain-value":              false,
		"doppler://prod/OTHER_KEY": true,
		"infisical://inf/a/KEY":    true,
		"doppler://bad":            true,
		"op://onep/Item/field":     true,
		"op://Personal/Item/field": false,
		"op://broken":              false,
	}
	for v, want := range cases {
		got, err := st.ResolvesLive(ctx, v)
		if err != nil || got != want {
			t.Errorf("ResolvesLive(%q) = %v, %v; want %v", v, got, err, want)
		}
	}
}
