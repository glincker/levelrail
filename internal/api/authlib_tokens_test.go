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

func TestAuthEngineStatus_RootOnly(t *testing.T) {
	rt, cookie, db := goldenRouter(t)
	if st := decodeStatus(t, rt, cookie); st.LibraryVersion == "" {
		t.Fatalf("status = %+v", st)
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

func TestLibraryModeBearerAuthentication(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	bootstrapTestAdmin(t, db)
	admin, err := db.GetUserByEmail(ctx, testAdminUsername)
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	system, _, err := MintAPIToken(ctx, db, "system", []string{AbilityRead}, nil)
	if err != nil {
		t.Fatal(err)
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
	rt := NewRouter(discardLogger(), testBrand(), db, WithAuthEngine(eng))
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
		{"ownerless system token", system, http.StatusOK},
		{"owned token the library never saw", late, http.StatusUnauthorized},
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
			rt, cookie, _ := goldenRouter(t)
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
