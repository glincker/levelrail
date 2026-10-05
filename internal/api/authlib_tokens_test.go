package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/authengine"
)

func decodeStatus(t *testing.T, rt *Router, cookie *http.Cookie) authEngineStatusResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/auth-engine/status", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status route: %d %s", rec.Code, rec.Body.String())
	}
	var out authEngineStatusResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return out
}

func TestAuthEngineStatus_RootOnlyAndMode(t *testing.T) {
	rt, cookie, db := goldenRouter(t, authengine.EngineLegacy)
	st := decodeStatus(t, rt, cookie)
	if st.Mode != authengine.EngineLegacy || st.Compared != 0 || st.LibraryVersion == "" || st.Mismatches == nil {
		t.Fatalf("legacy status = %+v", st)
	}
	reader := storeUserWithAbilitiesForTest(t, db, "reader@example.com", []string{AbilityRead})
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, sessionCookieForTest(t, rt, reader.ID), http.MethodGet, "/api/v1/auth-engine/status", ""))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-root status = %d, want 403", rec.Code)
	}
	anon := httptest.NewRecorder()
	rt.Handler().ServeHTTP(anon, httptest.NewRequest(http.MethodGet, "/api/v1/auth-engine/status", nil))
	if anon.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anon.Code)
	}
}

func TestShadowMismatchIsCountedAndSurfaced(t *testing.T) {
	rt, cookie, db := goldenRouter(t, authengine.EngineShadow)
	admin, err := db.GetUserByEmail(context.Background(), testAdminUsername)
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	raw, rec, err := MintAgentAPIToken(context.Background(), db, "legacy-only", []string{AbilityRead}, nil, agentIdentity{}, admin.ID)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	out := httptest.NewRecorder()
	rt.Handler().ServeHTTP(out, goldenBearer(http.MethodGet, raw, ""))
	if out.Code != http.StatusOK {
		t.Fatalf("legacy must keep serving in shadow mode, got %d", out.Code)
	}
	deadline := time.Now().Add(3 * time.Second)
	var st authEngineStatusResponse
	for time.Now().Before(deadline) {
		if st = decodeStatus(t, rt, cookie); st.Mismatched > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st.Mode != authengine.EngineShadow || st.Mismatched != 1 || st.Compared < 1 {
		t.Fatalf("status = %+v", st)
	}
	m := st.Mismatches[0]
	if m.Kind != authengine.MismatchDecision || m.TokenID != rec.ID || m.LegacyOwnerID != admin.ID {
		t.Fatalf("mismatch = %+v", m)
	}
	body, _ := json.Marshal(st)
	if strings.Contains(string(body), raw) {
		t.Fatal("status must never contain the token secret")
	}
}

func TestLibraryModeBearerAuthentication(t *testing.T) {
	t.Setenv(authengine.EnvEngine, authengine.EngineLibrary)
	t.Setenv(authengine.EnvAreas, "")
	ctx := context.Background()
	db := openTestDB(t)
	bootstrapTestAdmin(t, db)
	admin, err := db.GetUserByEmail(ctx, testAdminUsername)
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	owned, _, err := MintAgentAPIToken(ctx, db, "owned", []string{AbilityRead}, nil, agentIdentity{}, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	system, _, err := MintAPIToken(ctx, db, "system", []string{AbilityRead}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := authengine.Backfill(ctx, db.DB, authengine.BackfillOptions{}); err != nil {
		t.Fatalf("backfill: %v", err)
	}
	late, _, err := MintAgentAPIToken(ctx, db, "late", []string{AbilityRead}, nil, agentIdentity{}, admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	eng, err := authengine.New(db.DB, authengine.Config{
		BaseURL: "http://golden.test", TokenPrefix: "lt", Directory: authengine.NewDirectory(db.DB),
	})
	if err != nil {
		t.Fatalf("authengine.New: %v", err)
	}
	t.Cleanup(eng.Close)
	rt := NewRouter(discardLogger(), testBrand(), db, WithAuthEngineLibrary(eng))
	cookie := loginTestSession(t, rt, db)

	use := func(token string) int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, goldenBearer(http.MethodGet, token, ""))
		return rec.Code
	}
	tests := []struct {
		name  string
		token string
		want  int
	}{
		{"backfilled unprefixed legacy token", owned, http.StatusOK},
		{"ownerless system token", system, http.StatusOK},
		{"owned token the backfill never saw", late, http.StatusUnauthorized},
		{"unknown secret", "nope", http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := use(tc.token); got != tc.want {
				t.Fatalf("status = %d, want %d", got, tc.want)
			}
		})
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"new","abilities":["read"]}`))
	var created createTokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("mint: %d %s", rec.Code, rec.Body.String())
	}
	if !strings.HasPrefix(created.Token, "lt_") {
		t.Fatalf("new token %q must carry the library prefix", created.Token)
	}
	if got := use(created.Token); got != http.StatusOK {
		t.Fatalf("new token status = %d", got)
	}
	mirror, err := db.GetAPITokenByID(ctx, created.ID)
	if err != nil || mirror.OwnerUserID != admin.ID {
		t.Fatalf("legacy mirror = %+v, err = %v", mirror, err)
	}
}

func TestLibraryDeviceGrantRootCapAndTTL(t *testing.T) {
	tests := []struct {
		name      string
		allowRoot string
		want      []string
	}{
		{"root approver capped by default", "", []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive, AbilityDeploy}},
		{"root allowed by env", "true", []string{AbilityRoot}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envDeviceTokenAllowRoot, tc.allowRoot)
			t.Setenv(envDeviceTokenTTLDays, "2")
			rt, cookie, _ := goldenRouter(t, authengine.EngineLibrary)
			g := &goldenRun{t: t, rt: rt, cookie: cookie}
			start := g.do("start", httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/start", strings.NewReader(`{}`)))
			g.do("approve", g.session(http.MethodPost, "/api/v1/auth/device/"+start["user_code"].(string)+"/approve", ""))
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/token", strings.NewReader(`{"device_code":"`+start["device_code"].(string)+`"}`)))
			var tok createTokenResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &tok); err != nil || rec.Code != http.StatusOK {
				t.Fatalf("redeem: %d %s", rec.Code, rec.Body.String())
			}
			if !slices.Equal(tok.Abilities, tc.want) {
				t.Fatalf("abilities = %v, want %v", tok.Abilities, tc.want)
			}
			if tok.ExpiresAt == nil || time.Until(*tok.ExpiresAt) > 49*time.Hour || time.Until(*tok.ExpiresAt) < 47*time.Hour {
				t.Fatalf("expires_at = %v, want about 2 days", tok.ExpiresAt)
			}
			again := httptest.NewRecorder()
			rt.Handler().ServeHTTP(again, httptest.NewRequest(http.MethodPost, "/api/v1/auth/device/token", strings.NewReader(`{"device_code":"`+start["device_code"].(string)+`"}`)))
			if again.Code != http.StatusBadRequest {
				t.Fatalf("second redeem = %d, want 400", again.Code)
			}
		})
	}
}
