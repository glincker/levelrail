package cloudconn

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/secretsource"
)

// fakeSSO serves the OIDC and portal endpoints on one test server.
type fakeSSO struct {
	srv         *httptest.Server
	credCalls   atomic.Int32
	tokenCalls  atomic.Int32
	refreshFail atomic.Bool
	reject401   atomic.Bool
	pendingLeft atomic.Int32
	lifetimeMS  int64
}

func newFakeSSO(t *testing.T) *fakeSSO {
	f := &fakeSSO{lifetimeMS: 3600_000}
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(v)
	}
	mux.HandleFunc("/client/register", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"clientId": "cid", "clientSecret": "csecret", "clientSecretExpiresAt": time.Now().Add(24 * time.Hour).Unix()})
	})
	mux.HandleFunc("/device_authorization", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"deviceCode": "dev", "userCode": "ABCD-EFGH", "verificationUri": "https://v", "verificationUriComplete": "https://v?c=1", "expiresIn": 600, "interval": 1})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.tokenCalls.Add(1)
		if in["grantType"] == ssoGrantRefresh {
			if f.refreshFail.Load() {
				write(w, 400, map[string]string{"error": "invalid_grant"})
				return
			}
			write(w, 200, map[string]any{"accessToken": "access-2", "refreshToken": "refresh-2", "expiresIn": 28800})
			return
		}
		if f.pendingLeft.Add(-1) >= 0 {
			write(w, 400, map[string]string{"error": "authorization_pending"})
			return
		}
		write(w, 200, map[string]any{"accessToken": "access-1", "refreshToken": "refresh-1", "expiresIn": 28800})
	})
	mux.HandleFunc("/federation/credentials", func(w http.ResponseWriter, r *http.Request) {
		f.credCalls.Add(1)
		if r.Header.Get("x-amz-sso_bearer_token") == "" || r.Header.Get("Authorization") != "" {
			write(w, 401, map[string]string{"message": "no"})
			return
		}
		if f.reject401.Load() && r.Header.Get("x-amz-sso_bearer_token") == "access-1" {
			write(w, 401, map[string]string{"message": "expired"})
			return
		}
		write(w, 200, map[string]any{"roleCredentials": map[string]any{"accessKeyId": "AKIA", "secretAccessKey": "sk", "sessionToken": "st", "expiration": time.Now().UnixMilli() + f.lifetimeMS}})
	})
	mux.HandleFunc("/assignment/accounts", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"accountList": []map[string]string{{"accountId": "111122223333", "accountName": "prod", "emailAddress": "a@b.c"}}})
	})
	mux.HandleFunc("/assignment/roles", func(w http.ResponseWriter, _ *http.Request) {
		write(w, 200, map[string]any{"roleList": []map[string]string{{"roleName": "Admin"}}})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSSO) client() *SSOClient {
	return &SSOClient{HTTP: f.srv.Client(), OIDCBase: func(string) string { return f.srv.URL }, PortalBase: func(string) string { return f.srv.URL }}
}

func ssoConn() Connection {
	return Connection{Name: "acct", Provider: ProviderAWS, AuthMethod: AuthAWSSSO,
		Config: Config{Region: "eu-west-1", SSORegion: "eu-west-1", StartURL: "https://d-1.awsapps.com/start", AccountID: "111122223333", RoleName: "Admin"}}
}

func bundleJSON(b SSOBundle) []byte {
	raw, _ := json.Marshal(b) //nolint:gosec // test fixture
	return raw
}

func TestSSODeviceFlowAndListing(t *testing.T) {
	f := newFakeSSO(t)
	f.pendingLeft.Store(1)
	c := f.client()
	ctx := context.Background()
	reg, err := c.Register(ctx, "eu-west-1", "https://d-1.awsapps.com/start", "levelrail")
	if err != nil || reg.ClientID != "cid" {
		t.Fatalf("register: %v", err)
	}
	dev, err := c.StartDevice(ctx, "eu-west-1", "https://d-1.awsapps.com/start", reg)
	if err != nil || dev.UserCode != "ABCD-EFGH" {
		t.Fatalf("device: %v", err)
	}
	if _, err := c.DeviceToken(ctx, "eu-west-1", reg, dev.DeviceCode, time.Now()); !errors.Is(err, ErrSSOPending) {
		t.Fatalf("want pending, got %v", err)
	}
	tok, err := c.DeviceToken(ctx, "eu-west-1", reg, dev.DeviceCode, time.Now())
	if err != nil || tok.AccessToken != "access-1" || tok.RefreshToken != "refresh-1" {
		t.Fatalf("token: %v %+v", err, tok)
	}
	accts, err := c.ListAccounts(ctx, "eu-west-1", tok.AccessToken, 10)
	if err != nil || len(accts) != 1 || accts[0].AccountID != "111122223333" {
		t.Fatalf("accounts: %v %+v", err, accts)
	}
	if roles, err := c.ListRoles(ctx, "eu-west-1", tok.AccessToken, "111122223333"); err != nil || roles[0] != "Admin" {
		t.Fatalf("roles: %v %v", err, roles)
	}
}

func TestSSOBrokerCachesAndRefreshesAtExpiry(t *testing.T) {
	f := newFakeSSO(t)
	b := NewSSOBroker(f.client())
	now := time.Now()
	b.now = func() time.Time { return now }
	var mu sync.Mutex
	var saved []SSOBundle
	b.SetPersist(func(_ context.Context, _ string, raw []byte) error {
		var nb SSOBundle
		_ = json.Unmarshal(raw, &nb)
		mu.Lock()
		saved = append(saved, nb)
		mu.Unlock()
		return nil
	})
	seed := bundleJSON(SSOBundle{ClientID: "cid", ClientSecret: "s", AccessToken: "access-1", AccessExpiresAt: now.Add(time.Hour).Unix(), RefreshToken: "refresh-1", UpdatedAt: 1})
	for range 3 {
		if _, err := b.Credentials(context.Background(), ssoConn(), seed); err != nil {
			t.Fatal(err)
		}
	}
	if n := f.credCalls.Load(); n != 1 {
		t.Fatalf("keys must be cached, got %d portal calls", n)
	}
	now = now.Add(49 * time.Minute) // past 80 percent of the 1h role credentials
	if _, err := b.Credentials(context.Background(), ssoConn(), seed); err != nil {
		t.Fatal(err)
	}
	if f.credCalls.Load() != 2 || f.tokenCalls.Load() != 0 {
		t.Fatalf("renewal should only re-read keys while the access token lives")
	}
	now = now.Add(2 * time.Hour) // access token expired
	if _, err := b.Credentials(context.Background(), ssoConn(), seed); err != nil {
		t.Fatal(err)
	}
	if f.tokenCalls.Load() != 1 || len(saved) != 1 || saved[0].AccessToken != "access-2" {
		t.Fatalf("expected one refresh persisted, tokens=%d saved=%d", f.tokenCalls.Load(), len(saved))
	}
}

func TestSSOBrokerNeedsReloginWhenRefreshEnds(t *testing.T) {
	f := newFakeSSO(t)
	f.refreshFail.Store(true)
	b := NewSSOBroker(f.client())
	var persisted SSOBundle
	b.SetPersist(func(_ context.Context, _ string, raw []byte) error { return json.Unmarshal(raw, &persisted) })
	expired := bundleJSON(SSOBundle{ClientID: "cid", ClientSecret: "s", AccessToken: "old", AccessExpiresAt: time.Now().Add(-time.Hour).Unix(), RefreshToken: "r", UpdatedAt: 1})
	_, err := b.Credentials(context.Background(), ssoConn(), expired)
	if !errors.Is(err, ErrSSONeedsRelogin) || !persisted.NeedsRelogin {
		t.Fatalf("want needs_relogin persisted, got %v %+v", err, persisted)
	}
	// The in-memory bundle wins over the stale sealed one, so no second refresh call happens.
	calls := f.tokenCalls.Load()
	if _, err := b.Credentials(context.Background(), ssoConn(), expired); !errors.Is(err, ErrSSONeedsRelogin) || f.tokenCalls.Load() != calls {
		t.Fatalf("relogin state must stick without more calls: %v", err)
	}
	if !SSONeedsRelogin(bundleJSON(persisted), time.Now()) {
		t.Fatal("list state should flag the account")
	}
}

func TestSSOBrokerNoRefreshTokenAndUnauthorizedRetry(t *testing.T) {
	f := newFakeSSO(t)
	b := NewSSOBroker(f.client())
	noRefresh := bundleJSON(SSOBundle{ClientID: "cid", ClientSecret: "s", AccessToken: "old", AccessExpiresAt: time.Now().Add(-time.Minute).Unix(), UpdatedAt: 1})
	if _, err := b.Credentials(context.Background(), ssoConn(), noRefresh); !errors.Is(err, ErrSSONeedsRelogin) {
		t.Fatalf("no refresh token must need relogin, got %v", err)
	}
	f.reject401.Store(true)
	b2 := NewSSOBroker(f.client())
	live := bundleJSON(SSOBundle{ClientID: "cid", ClientSecret: "s", AccessToken: "access-1", AccessExpiresAt: time.Now().Add(time.Hour).Unix(), RefreshToken: "r", UpdatedAt: 1})
	if _, err := b2.Credentials(context.Background(), ssoConn(), live); err != nil {
		t.Fatalf("a 401 should refresh once and retry: %v", err)
	}
}

func TestSSOValidateConfig(t *testing.T) {
	p := NewAWSProvider(AWSOptions{})
	good := ssoConn().Config
	if err := p.ValidateConfig(good, AuthAWSSSO); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.StartURL = "http://x"
	if p.ValidateConfig(bad, AuthAWSSSO) == nil {
		t.Fatal("http start url must be rejected")
	}
	bad = good
	bad.RoleName = ""
	if p.ValidateConfig(bad, AuthAWSSSO) == nil {
		t.Fatal("role name is required")
	}
}

func TestSSOProviderCheckClassNeedsRelogin(t *testing.T) {
	f := newFakeSSO(t)
	p := NewAWSProvider(AWSOptions{SSO: NewSSOBroker(f.client())})
	raw := bundleJSON(SSOBundle{ClientID: "cid", ClientSecret: "s", NeedsRelogin: true})
	_, class, err := p.SSOKeys(context.Background(), Target{Connection: ssoConn(), Credential: secretsource.NewValue(raw, "")})
	if class != ClassNeedsRelogin || !errors.Is(err, ErrSSONeedsRelogin) {
		t.Fatalf("class=%s err=%v", class, err)
	}
}
