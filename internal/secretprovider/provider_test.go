package secretprovider

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
)

const fakeToken = "fake-provider-token-for-tests" //nolint:gosec // test fixture, not a credential

func tunables() Tunables {
	return Tunables{Timeout: 2 * time.Second, CacheTTL: time.Minute, MaxResponseBytes: 1 << 16}
}

func selfHosted(url string) Config {
	return Config{Name: "inf", Type: TypeInfisical, Project: "ws1", Environment: "prod", BaseURL: url}
}

func TestSecretProviderResolveOK(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer "+fakeToken || r.URL.Path != "/api/v3/secrets/raw/API_KEY" || r.URL.Query().Get("secretPath") != "/billing" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"secret":{"secretKey":"API_KEY","secretValue":"live-value"}}`))
	}))
	defer srv.Close()
	ref, ok, err := ParseRef("infisical://inf/billing/API_KEY")
	if !ok || err != nil {
		t.Fatalf("parse: %v %v", ok, err)
	}
	r := NewResolver()
	for i := 0; i < 2; i++ {
		v, err := r.Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tunables())
		if err != nil || v != "live-value" {
			t.Fatalf("fetch = %q, %v", v, err)
		}
	}
	if calls != 1 {
		t.Fatalf("cache should serve the second read, provider saw %d calls", calls)
	}
}

func TestSecretProviderDopplerOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.URL.Path != "/v3/configs/config/secret" || q.Get("project") != "web" || q.Get("config") != "prd" || q.Get("name") != "DB_URL" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"name":"DB_URL","value":{"raw":"raw","computed":"computed"}}`))
	}))
	defer srv.Close()
	r := NewResolver()
	r.defaultBase = map[string]string{TypeDoppler: srv.URL}
	cfg := Config{Name: "dp", Type: TypeDoppler, Project: "web", Environment: "prd"}
	ref, _, _ := ParseRef("doppler://dp/DB_URL")
	// The public host is not self-hosted, so the loopback test server must be refused.
	if _, err := r.Fetch(context.Background(), cfg, fakeToken, ref, tunables()); !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("loopback dial for a public provider must be blocked, got %v", err)
	}
}

func TestSecretProviderUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	ref, _, _ := ParseRef("infisical://inf/API_KEY")
	_, err := NewResolver().Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tunables())
	if !errors.Is(err, ErrUnauthorized) || strings.Contains(err.Error(), fakeToken) {
		t.Fatalf("want unauthorized without the token in the error, got %v", err)
	}
}

func TestSecretProviderTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()
	ref, _, _ := ParseRef("infisical://inf/API_KEY")
	tun := tunables()
	tun.Timeout = 100 * time.Millisecond
	_, err := NewResolver().Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tun)
	if err == nil || !strings.Contains(err.Error(), "did not answer") {
		t.Fatalf("want timeout reason, got %v", err)
	}
}

func TestSecretProviderRedirectBlocked(t *testing.T) {
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"secret":{"secretValue":"stolen"}}`))
	}))
	defer other.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, strings.Replace(other.URL, "127.0.0.1", "localhost", 1)+r.URL.Path, http.StatusFound) //nolint:gosec // test server, the redirect target is the point of the test
	}))
	defer srv.Close()
	ref, _, _ := ParseRef("infisical://inf/API_KEY")
	_, err := NewResolver().Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tunables())
	if !errors.Is(err, ErrRedirectBlocked) {
		t.Fatalf("want redirect blocked, got %v", err)
	}
}

func TestSecretProviderPrivateIPBlocked(t *testing.T) {
	for _, ip := range []string{"10.0.0.5", "127.0.0.1", "192.168.1.1", "169.254.169.254"} {
		if !blockedIP(parseIP(t, ip), false) {
			t.Fatalf("%s must be blocked for a public provider", ip)
		}
	}
	if !blockedIP(parseIP(t, "169.254.169.254"), true) {
		t.Fatal("link-local metadata must stay blocked even for a self-hosted URL")
	}
	if blockedIP(parseIP(t, "10.0.0.5"), true) {
		t.Fatal("an admin set self-hosted URL may use a private address")
	}
}

func TestSecretProviderConfigValidate(t *testing.T) {
	bad := []Config{
		{Name: "Bad", Type: TypeDoppler, Project: "p", Environment: "e"},
		{Name: "dp", Type: TypeDoppler, Project: "p", Environment: "e", BaseURL: "https://x.example"},
		{Name: "inf", Type: TypeInfisical, Project: "p", Environment: "e", BaseURL: "https://user:pw@x.example"}, //nolint:gosec // fake userinfo that must be rejected
		{Name: "x", Type: "vault", Project: "p", Environment: "e"},
	}
	for _, c := range bad {
		if c.Validate() == nil {
			t.Fatalf("config %+v must be rejected", c)
		}
	}
	if _, _, err := ParseRef("doppler://dp/a/B"); err == nil {
		t.Fatal("doppler reference with a path must be rejected")
	}
}

func TestSecretProviderEnvResolverFailsClosed(t *testing.T) {
	inner := memSecrets{"app": {"API_KEY": "doppler://missing/API_KEY", "PLAIN": "x"}}
	e := EnvResolver{Inner: inner, Providers: Store{Secrets: inner}, Resolver: NewResolver(), Tunables: func(context.Context) Tunables { return tunables() }}
	if v, err := e.Resolve(context.Background(), "app", "PLAIN"); err != nil || v != "x" {
		t.Fatalf("plain secret = %q, %v", v, err)
	}
	if _, err := e.Resolve(context.Background(), "app", "API_KEY"); !errors.Is(err, ErrNoProvider) {
		t.Fatalf("missing provider must fail the read, got %v", err)
	}
}

func TestSecretProviderEnvResolverEndToEnd(t *testing.T) {
	var logs bytes.Buffer
	prevLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prevLogger)

	status, body := http.StatusOK, `{"secret":{"secretValue":"resolved-live-value-1"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	mem := memSecrets{"app": {"API_KEY": "infisical://inf/API_KEY"}}
	st := Store{Secrets: mem}
	if _, err := st.Put(context.Background(), selfHosted(srv.URL), fakeToken, 0, time.Now()); err != nil {
		t.Fatal(err)
	}
	tun := tunables()
	tun.CacheTTL = 0
	e := EnvResolver{Inner: mem, Providers: st, Resolver: NewResolver(), Tunables: func(context.Context) Tunables { return tun }}

	v, err := e.Resolve(context.Background(), "app", "API_KEY")
	if err != nil || v != "resolved-live-value-1" {
		t.Fatalf("resolve = %q, %v", v, err)
	}
	if masked := secretmask.Mask("log: " + v); strings.Contains(masked, v) {
		t.Fatalf("resolved value not registered with the masker: %q", masked)
	}

	var errs []string
	for _, c := range []struct {
		status int
		body   string
		want   string
	}{
		{http.StatusUnauthorized, ``, "rejected the token"},
		{http.StatusNotFound, ``, "not found"},
		{http.StatusOK, `{"secret":{"secretValue":""}}`, "is empty"},
		{http.StatusBadGateway, ``, "HTTP 502"},
	} {
		status, body = c.status, c.body
		v, err := e.Resolve(context.Background(), "app", "API_KEY")
		if err == nil || v != "" || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("status %d must fail closed with %q, got %q, %v", c.status, c.want, v, err)
		}
		errs = append(errs, err.Error())
	}
	srv.Close()
	if _, err := e.Resolve(context.Background(), "app", "API_KEY"); err == nil || !strings.Contains(err.Error(), "unreachable") {
		t.Fatalf("provider down must fail closed with a reason, got %v", err)
	} else {
		errs = append(errs, err.Error())
	}
	if strings.Contains(logs.String()+strings.Join(errs, "\n"), fakeToken) {
		t.Fatal("provider token leaked into a log line or error")
	}
}

func TestSecretProviderTunablesFromRuntimeSettings(t *testing.T) {
	got := TunablesFrom(nil)(context.Background())
	if got.Timeout != 10*time.Second || got.CacheTTL != time.Minute || got.MaxResponseBytes != 1024*1024 {
		t.Fatalf("defaults from the runtime settings registry = %+v", got)
	}
	t.Setenv("APP_SECRET_PROVIDER_TIMEOUT_SECONDS", "3")
	if got := TunablesFrom(nil)(context.Background()); got.Timeout != 3*time.Second {
		t.Fatalf("env seed not applied: %+v", got)
	}
}

func parseIP(t *testing.T, s string) net.IP {
	t.Helper()
	ip := net.ParseIP(s)
	if ip == nil {
		t.Fatalf("bad ip %q", s)
	}
	return ip
}

// memSecrets is an in-memory stand-in for internal/secrets.Manager.
type memSecrets map[string]map[string]string

func (m memSecrets) SetValue(_ context.Context, svc, key, v string) error {
	if m[svc] == nil {
		m[svc] = map[string]string{}
	}
	m[svc][key] = v
	return nil
}

func (m memSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	v, ok := m[svc][key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (m memSecrets) Exists(_ context.Context, svc, key string) (bool, error) {
	_, ok := m[svc][key]
	return ok, nil
}

func (m memSecrets) ListKeys(_ context.Context, svc string) ([]store.SecretKeyInfo, error) {
	var out []store.SecretKeyInfo
	for k := range m[svc] {
		out = append(out, store.SecretKeyInfo{Key: k})
	}
	return out, nil
}

func (m memSecrets) DeleteValue(_ context.Context, svc, key string) error {
	delete(m[svc], key)
	return nil
}

func TestSecretProviderStoreRotation(t *testing.T) {
	mem := memSecrets{}
	st := Store{Secrets: mem}
	now := time.Now()
	cfg := Config{Name: "dp", Type: TypeDoppler, Project: "p", Environment: "e"}
	if _, err := st.Put(context.Background(), cfg, "", time.Hour, now); err == nil {
		t.Fatal("first save without a token must fail")
	}
	if _, err := st.Put(context.Background(), cfg, "old", time.Hour, now); err != nil {
		t.Fatal(err)
	}
	saved, err := st.Put(context.Background(), cfg, "new", time.Hour, now)
	if err != nil || saved.GraceUntil.IsZero() {
		t.Fatalf("rotation must set a grace window: %+v %v", saved, err)
	}
	cur, prev, _ := st.Tokens(context.Background(), saved, now)
	if cur != "new" || prev != "old" {
		t.Fatalf("tokens = %q %q", cur, prev)
	}
	list, _ := st.List(context.Background())
	if len(list) != 1 || list[0].Name != "dp" {
		t.Fatalf("list = %+v", list)
	}
	if err := st.Delete(context.Background(), "dp"); err != nil || len(mem[OwnerKey]) != 0 {
		t.Fatalf("delete left %v (%v)", mem[OwnerKey], err)
	}
}

func onePassServer(t *testing.T, itemStatus int, itemBody string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+fakeToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/v1/vaults":
			_, _ = w.Write([]byte(`[{"id":"vaultid00000000000000000a1","name":"Prod"}]`))
		case "/v1/vaults/vaultid00000000000000000a1/items":
			_, _ = w.Write([]byte(`[{"id":"itemid000000000000000000b2","title":"Stripe"}]`))
		case "/v1/vaults/vaultid00000000000000000a1/items/itemid000000000000000000b2":
			w.WriteHeader(itemStatus)
			_, _ = w.Write([]byte(itemBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestSecretProviderOnePassword(t *testing.T) {
	op := func(url string) Config {
		return Config{Name: "op1", Type: TypeOnePassword, BaseURL: url}
	}
	ref, ok, err := ParseRef("op://op1/Prod/Stripe/api_key")
	if !ok || err != nil || ref.Vault != "Prod" || ref.Item != "Stripe" || ref.Name != "api_key" {
		t.Fatalf("parse: %+v %v %v", ref, ok, err)
	}
	good := `{"fields":[{"id":"x","label":"api_key","value":"op-live-value"}]}`
	srv := onePassServer(t, http.StatusOK, good)
	defer srv.Close()
	if v, err := NewResolver().Fetch(context.Background(), op(srv.URL), fakeToken, ref, tunables()); err != nil || v != "op-live-value" {
		t.Fatalf("fetch = %q, %v", v, err)
	}
	if _, err := NewResolver().Fetch(context.Background(), op(srv.URL), "wrong", ref, tunables()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("401 err = %v", err)
	}
	missing := onePassServer(t, http.StatusNotFound, "")
	defer missing.Close()
	if _, err := NewResolver().Fetch(context.Background(), op(missing.URL), fakeToken, ref, tunables()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404 err = %v", err)
	}
	big := onePassServer(t, http.StatusOK, good+strings.Repeat(" ", 1<<17))
	defer big.Close()
	if _, err := NewResolver().Fetch(context.Background(), op(big.URL), fakeToken, ref, tunables()); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("oversize err = %v", err)
	}
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://other.example.invalid/v1/vaults", http.StatusFound)
	}))
	defer redir.Close()
	if _, err := NewResolver().Fetch(context.Background(), op(redir.URL), fakeToken, ref, tunables()); !errors.Is(err, ErrRedirectBlocked) {
		t.Fatalf("redirect err = %v", err)
	}
	if err := op("").Validate(); err == nil {
		t.Fatal("onepassword without base_url must be rejected")
	}
}

func TestOnePasswordVaultScopeRefused(t *testing.T) {
	cfg := Config{Name: "op1", Type: TypeOnePassword, BaseURL: "http://127.0.0.1:1", Project: "Prod"}
	ref, _, err := ParseRef("op://op1/Infra/db/password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewResolver().Fetch(context.Background(), cfg, fakeToken, ref, tunables()); err == nil || !strings.Contains(err.Error(), "outside the provider") {
		t.Fatalf("err = %v", err)
	}
}

func TestOnePasswordNativeValuePassesThrough(t *testing.T) {
	native := "op://Prod/Stripe/api_key"
	five := "op://Prod/Stripe/section/field/extra"
	inner := memSecrets{"app": {"A": native, "B": five}}
	e := EnvResolver{Inner: inner, Providers: Store{Secrets: inner}, Resolver: NewResolver(), Tunables: func(context.Context) Tunables { return tunables() }}
	for k, want := range map[string]string{"A": native, "B": five} {
		if v, err := e.Resolve(context.Background(), "app", k); err != nil || v != want {
			t.Fatalf("%s = %q, %v", k, v, err)
		}
	}
}
