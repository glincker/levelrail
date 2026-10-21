package registrytoken

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestECRMint(t *testing.T) {
	expires := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Amz-Target") != ecrTarget {
			t.Errorf("target = %q", r.Header.Get("X-Amz-Target"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=AKIATEST/") {
			t.Errorf("request not signed: %q", r.Header.Get("Authorization"))
		}
		tok := base64.StdEncoding.EncodeToString([]byte("AWS:ecr-pass"))
		_, _ = fmt.Fprintf(w, `{"authorizationData":[{"authorizationToken":%q,"expiresAt":%d,"proxyEndpoint":"https://123456789012.dkr.ecr.eu-west-1.amazonaws.com"}]}`, tok, expires.Unix())
	}))
	defer srv.Close()

	e := &ECR{Client: srv.Client(), Endpoint: srv.URL, Now: func() time.Time { return expires }}
	got, err := e.Mint(context.Background(), `{"access_key_id":"AKIATEST","secret_access_key":"s3cret","region":"eu-west-1"}`, Config{})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if got.Username != "AWS" || got.Password != "ecr-pass" || got.RegistryHost != "123456789012.dkr.ecr.eu-west-1.amazonaws.com" || !got.ExpiresAt.Equal(expires) {
		t.Fatalf("unexpected login: %+v", got)
	}
}

func TestECRMintRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"__type":"AccessDeniedException"}`)
	}))
	defer srv.Close()
	e := &ECR{Client: srv.Client(), Endpoint: srv.URL}
	_, err := e.Mint(context.Background(), `{"access_key_id":"AKIATEST","secret_access_key":"s3cret"}`, Config{})
	if err == nil || !strings.Contains(err.Error(), "403") || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("err = %v", err)
	}
}

func TestArtifactRegistryMint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.Form.Get("assertion") == "" {
			t.Errorf("missing jwt assertion: %v", err)
		}
		_, _ = io.WriteString(w, `{"access_token":"ya29.fake","token_type":"Bearer","expires_in":3600}`)
	}))
	defer srv.Close()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: mustPKCS8(t, key)})
	raw, _ := json.Marshal(map[string]string{
		"type": "service_account", "project_id": "p", "private_key_id": "k",
		"private_key": string(pemKey), "client_email": "sa@p.iam.gserviceaccount.com", "token_uri": srv.URL,
	})

	got, err := (&ArtifactRegistry{Client: srv.Client()}).Mint(context.Background(), string(raw), Config{Region: "europe"})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if got.Username != "oauth2accesstoken" || got.Password != "ya29.fake" || got.RegistryHost != "europe-docker.pkg.dev" || time.Until(got.ExpiresAt) < 50*time.Minute {
		t.Fatalf("unexpected login: %+v", got)
	}
}

func mustPKCS8(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	b, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return b
}

func TestACRMint(t *testing.T) {
	exp := time.Now().Add(3 * time.Hour).Truncate(time.Second)
	payload, _ := json.Marshal(map[string]int64{"exp": exp.Unix()})
	refresh := "h." + base64.RawURLEncoding.EncodeToString(payload) + ".s"

	aad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/tenant-1/oauth2/v2.0/token") {
			t.Errorf("aad path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"aad-token","token_type":"Bearer","expires_in":3600}`)
	}))
	defer aad.Close()
	acr := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.URL.Path != "/oauth2/exchange" || r.Form.Get("access_token") != "aad-token" || r.Form.Get("grant_type") != "access_token" {
			t.Errorf("bad exchange: %s %v", r.URL.Path, r.Form)
		}
		_, _ = fmt.Fprintf(w, `{"refresh_token":%q}`, refresh)
	}))
	defer acr.Close()

	m := &ACR{Client: acr.Client(), LoginBase: aad.URL, ExchangeBase: acr.URL, FallbackTTL: time.Hour}
	got, err := m.Mint(context.Background(),
		`{"tenant_id":"tenant-1","client_id":"c","client_secret":"x","subscription_id":"s","resource_group":"rg"}`,
		Config{RegistryHost: "demo.azurecr.io"})
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if got.Username != acrUsername || got.Password != refresh || got.RegistryHost != "demo.azurecr.io" || !got.ExpiresAt.Equal(exp) {
		t.Fatalf("unexpected login: %+v", got)
	}
}

func TestACRRequiresHost(t *testing.T) {
	if _, err := (&ACR{}).Mint(context.Background(), `{}`, Config{}); err == nil {
		t.Fatal("expected error for missing host")
	}
}

type fakeStore struct {
	cred store.RegistryCredential
}

func (f *fakeStore) ListRegistryCredentials(context.Context) ([]store.RegistryCredential, error) {
	return []store.RegistryCredential{f.cred}, nil
}

func (f *fakeStore) GetRegistryCredential(context.Context, string) (store.RegistryCredential, error) {
	return f.cred, nil
}

func (f *fakeStore) RecordRegistryCredentialRefresh(_ context.Context, _, host, user string, exp, refreshed, next *time.Time, errMsg string) error {
	if host != "" {
		f.cred.RegistryHost = host
	}
	if user != "" {
		f.cred.Username = user
	}
	if exp != nil {
		f.cred.ExpiresAt = exp
	}
	if refreshed != nil {
		f.cred.LastRefreshedAt = refreshed
	}
	f.cred.NextRefreshAt = next
	f.cred.LastRefreshError = errMsg
	return nil
}

type fakeSecrets struct {
	values map[string]string
	writes []string
}

func (f *fakeSecrets) SetValue(_ context.Context, svc, key, v string) error {
	f.values[svc+"/"+key] = v
	f.writes = append(f.writes, v)
	return nil
}

func (f *fakeSecrets) Resolve(_ context.Context, svc, key string) (string, error) {
	v, ok := f.values[svc+"/"+key]
	if !ok {
		return "", secrets.ErrValueNotFound
	}
	return v, nil
}

type stubMinter struct {
	calls int
	err   error
	ttl   time.Duration
	now   func() time.Time
}

func (s *stubMinter) Mint(context.Context, string, Config) (Minted, error) {
	s.calls++
	if s.err != nil {
		return Minted{}, s.err
	}
	return Minted{RegistryHost: "h.example", Username: "AWS", Password: fmt.Sprintf("pw-%d", s.calls), ExpiresAt: s.now().Add(s.ttl)}, nil
}

func newTestRefresher(now *time.Time, m *stubMinter) (*Refresher, *fakeStore, *fakeSecrets) {
	st := &fakeStore{cred: store.RegistryCredential{ID: "rc1", Name: "n", Preset: PresetECR, RegistryHost: "h.example"}}
	sec := &fakeSecrets{values: map[string]string{store.NodeProviderSecretsKey("aws") + "/token": "{}"}}
	m.now = func() time.Time { return *now }
	return &Refresher{
		Store: st, Secrets: sec, Minters: map[string]Minter{PresetECR: m},
		Settings: Settings{Lead: time.Hour, Retry: 5 * time.Minute},
		Now:      func() time.Time { return *now },
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}, st, sec
}

func TestTickSchedulesWithFakeClock(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &stubMinter{ttl: 12 * time.Hour}
	r, st, sec := newTestRefresher(&now, m)
	ctx := context.Background()

	if err := r.Tick(ctx); err != nil || m.calls != 1 {
		t.Fatalf("first tick: err=%v calls=%d", err, m.calls)
	}
	if want := now.Add(11 * time.Hour); !st.cred.NextRefreshAt.Equal(want) {
		t.Fatalf("next refresh = %v, want %v", st.cred.NextRefreshAt, want)
	}
	if got := sec.values[store.RegistryCredentialSecretsKey("rc1")+"/password"]; got != "pw-1" {
		t.Fatalf("stored password = %q", got)
	}

	now = now.Add(10 * time.Hour)
	_ = r.Tick(ctx)
	if m.calls != 1 {
		t.Fatalf("refreshed early, calls=%d", m.calls)
	}
	now = now.Add(time.Hour)
	_ = r.Tick(ctx)
	if m.calls != 2 || st.cred.LastRefreshedAt == nil || !st.cred.LastRefreshedAt.Equal(now) {
		t.Fatalf("expected second refresh at lead time, calls=%d", m.calls)
	}
}

func TestShortLivedTokenRefreshesAtHalfLife(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &stubMinter{ttl: 30 * time.Minute}
	r, st, _ := newTestRefresher(&now, m)
	if _, err := r.Refresh(context.Background(), "rc1"); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if want := now.Add(15 * time.Minute); !st.cred.NextRefreshAt.Equal(want) {
		t.Fatalf("next = %v, want %v", st.cred.NextRefreshAt, want)
	}
}

func TestFailureKeepsOldPasswordAndRecordsError(t *testing.T) {
	now := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	m := &stubMinter{ttl: 12 * time.Hour}
	r, st, sec := newTestRefresher(&now, m)
	ctx := context.Background()
	if _, err := r.Refresh(ctx, "rc1"); err != nil {
		t.Fatalf("seed refresh: %v", err)
	}
	firstExpiry := *st.cred.ExpiresAt

	m.err = errors.New("access denied")
	now = now.Add(11 * time.Hour)
	if _, err := r.Refresh(ctx, "rc1"); err == nil {
		t.Fatal("expected error")
	}
	if len(sec.writes) != 1 || sec.values[store.RegistryCredentialSecretsKey("rc1")+"/password"] != "pw-1" {
		t.Fatalf("password changed on failure: %v", sec.writes)
	}
	if !strings.Contains(st.cred.LastRefreshError, "access denied") {
		t.Fatalf("error not recorded: %q", st.cred.LastRefreshError)
	}
	if !st.cred.ExpiresAt.Equal(firstExpiry) || !st.cred.NextRefreshAt.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("expiry/next wrong: %v %v", st.cred.ExpiresAt, st.cred.NextRefreshAt)
	}

	m.err = nil
	now = now.Add(5 * time.Minute)
	if err := r.Tick(ctx); err != nil || st.cred.LastRefreshError != "" {
		t.Fatalf("recovery failed: err=%v lastErr=%q", err, st.cred.LastRefreshError)
	}
}

func TestMissingCloudConnection(t *testing.T) {
	now := time.Now()
	r, st, sec := newTestRefresher(&now, &stubMinter{ttl: time.Hour})
	delete(sec.values, store.NodeProviderSecretsKey("aws")+"/token")
	if _, err := r.Refresh(context.Background(), "rc1"); !errors.Is(err, ErrNoCloudConnection) {
		t.Fatalf("err = %v", err)
	}
	if st.cred.LastRefreshError == "" {
		t.Fatal("failure not recorded")
	}
}
