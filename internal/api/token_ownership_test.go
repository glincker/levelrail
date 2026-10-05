package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

func mintTokenAs(t *testing.T, rt *Router, cookie *http.Cookie, name string) createTokenResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"`+name+`","abilities":["read"]}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint %q: status = %d, body = %s", name, rec.Code, rec.Body.String())
	}
	var out createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode mint: %v", err)
	}
	return out
}

func listTokenIDs(t *testing.T, rt *Router, cookie *http.Cookie) []string {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth/tokens", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("list: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got []tokenResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	ids := make([]string, 0, len(got))
	for _, tr := range got {
		ids = append(ids, tr.ID)
	}
	return ids
}

func TestTokenOwnership_ListAndRevokeScopedToOwner(t *testing.T) {
	rt, db := newTestRouter(t)
	adminCookie := loginTestSession(t, rt, db)
	alice := storeUserWithAbilitiesForTest(t, db, "alice@example.com", []string{AbilityRead})
	bob := storeUserWithAbilitiesForTest(t, db, "bob@example.com", []string{AbilityRead})
	aliceCookie := sessionCookieForTest(t, rt, alice.ID)
	bobCookie := sessionCookieForTest(t, rt, bob.ID)

	aliceTok := mintTokenAs(t, rt, aliceCookie, "alice-tok")
	bobTok := mintTokenAs(t, rt, bobCookie, "bob-tok")

	tests := []struct {
		name    string
		cookie  *http.Cookie
		wantIDs []string
		wantAll bool
	}{
		{"owner sees only own", aliceCookie, []string{aliceTok.ID}, false},
		{"other owner sees only own", bobCookie, []string{bobTok.ID}, false},
		{"admin sees all", adminCookie, []string{aliceTok.ID, bobTok.ID}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ids := listTokenIDs(t, rt, tc.cookie)
			if !tc.wantAll && len(ids) != len(tc.wantIDs) {
				t.Fatalf("ids = %v, want exactly %v", ids, tc.wantIDs)
			}
			for _, want := range tc.wantIDs {
				if !slices.Contains(ids, want) {
					t.Errorf("ids = %v, missing %s", ids, want)
				}
			}
		})
	}

	crossRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(crossRec, authedRequest(t, bobCookie, http.MethodDelete, "/api/v1/auth/tokens/"+aliceTok.ID, ""))
	if crossRec.Code != http.StatusNotFound {
		t.Fatalf("bob revoking alice's token: status = %d, want 404", crossRec.Code)
	}
	got, err := db.GetAPITokenByID(context.Background(), aliceTok.ID)
	if err != nil || got.RevokedAt != nil {
		t.Fatalf("alice's token must stay live after cross-user revoke, got err=%v rec=%+v", err, got)
	}

	adminRec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(adminRec, authedRequest(t, adminCookie, http.MethodDelete, "/api/v1/auth/tokens/"+aliceTok.ID, ""))
	if adminRec.Code != http.StatusNoContent {
		t.Fatalf("admin revoke: status = %d, want 204", adminRec.Code)
	}
}

func TestTokenOwnership_DeletedOwnerInvalidatesToken(t *testing.T) {
	rt, db := newTestRouter(t)
	alice := storeUserWithAbilitiesForTest(t, db, "alice@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, alice.ID)
	tok := mintTokenAs(t, rt, cookie, "alice-tok")

	use := func() int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		return rec.Code
	}
	if code := use(); code != http.StatusOK {
		t.Fatalf("token before owner deletion: status = %d, want 200", code)
	}
	if err := db.DeleteUser(context.Background(), alice.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if code := use(); code != http.StatusUnauthorized {
		t.Fatalf("token after owner deletion: status = %d, want 401", code)
	}
}

func TestSessionLink_PinnedSessionFollowsTokenRevocation(t *testing.T) {
	rt, db := newTestRouter(t)
	plain := seedRootAPIToken(t, db, "tok_pinned", []string{AbilityRoot})
	minted, rec := mintSessionLink(t, rt, nil, plain)
	if rec.Code != http.StatusCreated {
		t.Fatalf("mint: status = %d", rec.Code)
	}
	cookie := sessionCookieFromRecorder(t, consumeSessionLinkRequest(t, rt, minted.Token))

	get := func() int {
		r := httptest.NewRecorder()
		rt.Handler().ServeHTTP(r, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps", ""))
		return r.Code
	}
	if code := get(); code != http.StatusOK {
		t.Fatalf("pinned session before revoke: status = %d, want 200", code)
	}
	if err := db.RevokeAPIToken(context.Background(), "tok_pinned"); err != nil {
		t.Fatalf("RevokeAPIToken: %v", err)
	}
	if code := get(); code != http.StatusUnauthorized {
		t.Fatalf("pinned session after token revoke: status = %d, want 401", code)
	}
}

func approveDevice(t *testing.T, rt *Router, cookie *http.Cookie, client string) deviceStartResponse {
	t.Helper()
	started := startDeviceAuth(t, rt, client)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/device/"+started.UserCode+"/approve", ""))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("approve: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return started
}

func TestDeviceAuth_ConcurrentPollsMintExactlyOneToken(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	started := approveDevice(t, rt, cookie, "racer")

	const pollers = 12
	var wg sync.WaitGroup
	var mu sync.Mutex
	granted := 0
	start := make(chan struct{})
	for i := 0; i < pollers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rec := pollDeviceToken(t, rt, started.DeviceCode)
			if rec.Code == http.StatusOK {
				mu.Lock()
				granted++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if granted != 1 {
		t.Fatalf("%d polls received a token, want exactly 1", granted)
	}
	toks, err := db.ListAPITokens(context.Background())
	if err != nil {
		t.Fatalf("ListAPITokens: %v", err)
	}
	cli := 0
	for _, tk := range toks {
		if strings.HasPrefix(tk.Name, "cli login") {
			cli++
		}
	}
	if cli != 1 {
		t.Fatalf("%d cli login tokens persisted, want 1", cli)
	}
}

func TestDeviceAuth_TokenExpiresOwnedAndNotSilentlyRoot(t *testing.T) {
	tests := []struct {
		name      string
		allowRoot string
		wantRoot  bool
	}{
		{"default caps root", "", false},
		{"explicit opt-in keeps root", "true", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envDeviceTokenAllowRoot, tc.allowRoot)
			t.Setenv(envDeviceTokenTTLDays, "7")
			rt, db := newTestRouter(t)
			cookie := loginTestSession(t, rt, db)
			started := approveDevice(t, rt, cookie, "laptop")

			rec := pollDeviceToken(t, rt, started.DeviceCode)
			if rec.Code != http.StatusOK {
				t.Fatalf("poll: status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var out createTokenResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if out.ExpiresAt == nil {
				t.Fatal("device token has no expiry")
			}
			if d := time.Until(*out.ExpiresAt); d < 6*24*time.Hour || d > 7*24*time.Hour+time.Minute {
				t.Errorf("expiry in %v, want about 7 days", d)
			}
			if got := slices.Contains(out.Abilities, AbilityRoot); got != tc.wantRoot {
				t.Errorf("abilities = %v, root = %v, want root = %v", out.Abilities, got, tc.wantRoot)
			}
			stored, err := db.GetAPITokenByID(context.Background(), out.ID)
			if err != nil || stored.OwnerUserID == "" {
				t.Errorf("device token owner not recorded: err=%v rec=%+v", err, stored)
			}
		})
	}
}
