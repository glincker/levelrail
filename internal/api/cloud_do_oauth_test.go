package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDOToken struct {
	mu       sync.Mutex
	exchange int
	refresh  int
	revoked  int
	expires  int
}

func (f *fakeDOToken) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	f.mu.Lock()
	defer f.mu.Unlock()
	switch r.URL.Path {
	case "/v1/oauth/token":
		if r.Form.Get("client_secret") != "sec" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		n := f.exchange + f.refresh + 1
		if r.Form.Get("grant_type") == "refresh_token" {
			if r.Form.Get("refresh_token") != "refresh-"+strconv.Itoa(n-1) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.refresh++
		} else {
			if r.Form.Get("code") != "good" || r.Form.Get("code_verifier") == "" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			f.exchange++
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access-" + strconv.Itoa(n), "refresh_token": "refresh-" + strconv.Itoa(n), "expires_in": f.expires})
	case "/v1/oauth/revoke":
		f.revoked++
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func doOAuthRouter(t *testing.T, fake *fakeDOToken) (*Router, *http.Cookie, *secretsHolder) {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)
	old := doOAuthBase
	doOAuthBase = srv.URL
	t.Cleanup(func() { doOAuthBase = old })
	secrets := newFakeNodeProviderSecrets()
	db := openTestDB(t)
	rt := NewRouter(slog.New(slog.NewTextHandler(discardWriter{}, nil)), testBrand(), db, WithNodeProviderSecrets(secrets))
	cookie := loginTestSession(t, rt, db)
	setPrimaryDomain(t, db)
	return rt, cookie, &secretsHolder{secrets}
}

type secretsHolder struct{ s NodeProviderSecrets }

func startDO(t *testing.T, rt *Router, cookie *http.Cookie) string {
	t.Helper()
	rec := provReq(rt, cookie, t, http.MethodGet, "/api/v1/cloud-accounts/oauth/digitalocean/start", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("start = %d %s", rec.Code, rec.Body.String())
	}
	var out doOAuthStartResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	u, err := url.Parse(out.URL)
	if err != nil || u.Query().Get("code_challenge_method") != "S256" || u.Query().Get("state") == "" {
		t.Fatalf("bad authorize url %q", out.URL)
	}
	return u.Query().Get("state")
}

func TestDigitalOceanOAuthGateAndFlow(t *testing.T) {
	fake := &fakeDOToken{expires: 3600}
	rt, cookie, _ := doOAuthRouter(t, fake)

	rec := provReq(rt, cookie, t, http.MethodGet, "/api/v1/cloud-accounts/oauth/digitalocean/start", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("unconfigured start = %d", rec.Code)
	}
	st := provReq(rt, cookie, t, http.MethodGet, "/api/v1/cloud-accounts/oauth/digitalocean/status", "")
	if !strings.Contains(st.Body.String(), `"enabled":false`) {
		t.Fatalf("status = %s", st.Body.String())
	}
	setSetting(t, rt, runtimesettings.KeyCloudOAuthDOClientID, `"cid"`)
	setSetting(t, rt, runtimesettings.KeyCloudOAuthDOClientSecret, `"sec"`)

	state := startDO(t, rt, cookie)
	cb := "/api/v1/cloud-accounts/oauth/digitalocean/callback?code=good&state="
	if rec := provReq(rt, cookie, t, http.MethodGet, cb+"tampered", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("tampered state = %d", rec.Code)
	}
	rec = provReq(rt, cookie, t, http.MethodGet, cb+state, "")
	if rec.Code != http.StatusFound {
		t.Fatalf("callback = %d %s", rec.Code, rec.Body.String())
	}
	if rec := provReq(rt, cookie, t, http.MethodGet, cb+state, ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("replayed state = %d", rec.Code)
	}

	list := provReq(rt, cookie, t, http.MethodGet, "/api/v1/cloud-accounts", "")
	var accts []struct {
		ID, Method string
		Linked     bool
	}
	_ = json.Unmarshal(list.Body.Bytes(), &accts)
	if len(accts) != 1 || accts[0].Method != "oauth" || !accts[0].Linked {
		t.Fatalf("accounts = %s", list.Body.String())
	}
	acct := store.CloudAccount{ID: accts[0].ID, Provider: "digitalocean", AuthMethod: "oauth", CredentialRef: cloudAccountSecretPref + accts[0].ID}

	_, _ = rt.runtimeSettings.Set(t.Context(), runtimesettings.KeyCloudOAuthDORefreshLead, json.RawMessage(`60`), "test")
	tok, err := rt.doOAuthAccessToken(t.Context(), acct)
	if err != nil || tok != "access-1" || fake.refresh != 0 {
		t.Fatalf("fresh token = %q %v refreshes %d", tok, err, fake.refresh)
	}
	// Force expiry, then race callers: one refresh, rotated token stored.
	cur, _, _ := rt.loadDOOAuthBundle(t.Context(), acct.CredentialRef)
	cur.ExpiresAt = 1
	if err := rt.saveDOOAuthBundle(t.Context(), acct.CredentialRef, cur); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got, err := rt.doOAuthAccessToken(t.Context(), acct); err != nil || got != "access-2" {
				t.Errorf("refreshed token = %q %v", got, err)
			}
		}()
	}
	wg.Wait()
	if fake.refresh != 1 {
		t.Fatalf("refresh calls = %d, want 1", fake.refresh)
	}

	if rec := provReq(rt, cookie, t, http.MethodDelete, "/api/v1/cloud-accounts/"+acct.ID, ""); rec.Code != http.StatusNoContent || fake.revoked != 2 {
		t.Fatalf("delete = %d revoked %d", rec.Code, fake.revoked)
	}
}

func TestDigitalOceanOAuthLegacyLayoutIsRead(t *testing.T) {
	fake := &fakeDOToken{expires: 3600}
	rt, _, sec := doOAuthRouter(t, fake)
	acct := store.CloudAccount{ID: "digitalocean-old", Provider: "digitalocean", AuthMethod: "oauth", CredentialRef: cloudAccountSecretPref + "digitalocean-old"}
	ctx := t.Context()
	_ = sec.s.SetValue(ctx, acct.CredentialRef, store.NodeProviderTokenEnvKey, "access-old")
	_ = sec.s.SetValue(ctx, acct.CredentialRef, doOAuthRefreshKey, "refresh-old")
	_ = sec.s.SetValue(ctx, acct.CredentialRef, doOAuthExpiresKey, strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10))
	if tok, err := rt.doOAuthAccessToken(ctx, acct); err != nil || tok != "access-old" {
		t.Fatalf("legacy token = %q %v", tok, err)
	}
}

func TestDigitalOceanOAuthCLIStartCompletesInBrowser(t *testing.T) {
	fake := &fakeDOToken{expires: 3600}
	rt, cookie, _ := doOAuthRouter(t, fake)
	setSetting(t, rt, runtimesettings.KeyCloudOAuthDOClientID, `"cid"`)
	setSetting(t, rt, runtimesettings.KeyCloudOAuthDOClientSecret, `"sec"`)
	probe := authedRequest(t, cookie, http.MethodGet, "/", "")
	owner, ok := rt.currentSessionUserID(probe)
	if !ok {
		t.Fatal("no session user")
	}
	plain := "do-oauth-cli-token"
	if err := rt.tokens.SaveAPIToken(t.Context(), store.APIToken{ID: "tok_cli", Name: "cli", TokenHash: hashToken(plain),
		Abilities: []string{AbilityRead, AbilityWrite, AbilityWriteSensitive, AbilityRoot}, OwnerUserID: owner}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, bearerRequest(http.MethodGet, "/api/v1/cloud-accounts/oauth/digitalocean/start", "", plain))
	if rec.Code != http.StatusOK {
		t.Fatalf("cli start = %d %s", rec.Code, rec.Body.String())
	}
	var out doOAuthStartResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	u, _ := url.Parse(out.URL)
	cb := "/api/v1/cloud-accounts/oauth/digitalocean/callback?code=good&state=" + u.Query().Get("state")
	if rec := provReq(rt, cookie, t, http.MethodGet, cb, ""); rec.Code != http.StatusFound {
		t.Fatalf("browser callback = %d %s", rec.Code, rec.Body.String())
	}
}
