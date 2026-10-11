package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const authGatePath = "/api/v1/apps/web/domains/app.example.com/auth-gate"

func TestDomainAuthGate_GetDefaultsOff(t *testing.T) {
	t.Setenv("APP_AUTH_GATE_TIMEOUT", "9")
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, authGatePath, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got domainAuthGateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Configured || got.Enabled || got.CheckPath != "/" || got.DefaultTimeoutSeconds != 9 {
		t.Errorf("got = %+v, want unconfigured, disabled, check path /, default timeout 9", got)
	}
}

func TestDomainAuthGate_SetGetClear(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	body := `{"enabled":true,"auth_url":"http://auth:9091","check_path":"/check","copy_headers":["x-user","X-Email","x-user"],"timeout_seconds":4}`
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, authGatePath, body))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got domainAuthGateResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Configured || !got.Enabled || got.AuthURL != "http://auth:9091" || got.CheckPath != "/check" ||
		!reflect.DeepEqual(got.CopyHeaders, []string{"X-User", "X-Email"}) || got.TimeoutSeconds != 4 {
		t.Errorf("PUT = %+v", got)
	}
	row, found, err := db.GetDomainAuthGate(context.Background(), "app.example.com")
	if err != nil || !found || !row.Enabled {
		t.Fatalf("stored row = %+v found %v err %v", row, found, err)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodDelete, authGatePath, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d", rec.Code)
	}
	if _, found, _ := db.GetDomainAuthGate(context.Background(), "app.example.com"); found {
		t.Error("row survived DELETE")
	}
}

func TestDomainAuthGate_RejectsInvalidInput(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	tests := []struct {
		name string
		body string
	}{
		{"missing url", `{"enabled":true}`},
		{"userinfo", `{"enabled":true,"auth_url":"https://u:p@auth.example.com"}`},
		{"non http scheme", `{"enabled":true,"auth_url":"file:///etc/passwd"}`},
		{"empty host", `{"enabled":true,"auth_url":"http://"}`},
		{"bad check path", `{"enabled":true,"auth_url":"http://auth","check_path":"check"}`},
		{"bad header", `{"enabled":true,"auth_url":"http://auth","copy_headers":["X User"]}`},
		{"reserved header", `{"enabled":true,"auth_url":"http://auth","copy_headers":["Cookie"]}`},
		{"too many headers", `{"enabled":true,"auth_url":"http://auth","copy_headers":["A","B","C","D","E","F","G","H","I","J","K","L","M","N","O","P","Q"]}`},
		{"negative timeout", `{"enabled":true,"auth_url":"http://auth","timeout_seconds":-1}`},
		{"huge timeout", `{"enabled":true,"auth_url":"http://auth","timeout_seconds":100000}`},
		{"not json", `nope`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, authGatePath, tt.body))
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
	if _, found, _ := db.GetDomainAuthGate(context.Background(), "app.example.com"); found {
		t.Error("an invalid request stored a row")
	}
}

func TestDomainAuthGate_DomainNotOwned(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/domains/other.example.com/auth-gate", `{"auth_url":"http://auth"}`))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestDomainAuthGate_WritesNeedRoot(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)
	ctx := context.Background()

	tokens := map[string][]string{
		"write-scoped-token":  {AbilityWrite},  //nolint:gosec // fake fixture
		"deploy-scoped-token": {AbilityDeploy}, //nolint:gosec // fake fixture
	}
	for plaintext, abilities := range tokens {
		if err := db.SaveAPIToken(ctx, store.APIToken{
			ID: "tok_" + abilities[0], Name: abilities[0], TokenHash: hashToken(plaintext), Abilities: abilities, CreatedAt: time.Now(),
		}); err != nil {
			t.Fatalf("seed token: %v", err)
		}
		for _, method := range []string{http.MethodPut, http.MethodDelete} {
			req := httptest.NewRequest(method, authGatePath, strings.NewReader(`{"enabled":true,"auth_url":"http://auth"}`))
			req.Header.Set("Authorization", "Bearer "+plaintext)
			rec := httptest.NewRecorder()
			rt.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusForbidden {
				t.Errorf("%s with %v = %d, want 403", method, abilities, rec.Code)
			}
		}
	}
	if _, found, _ := db.GetDomainAuthGate(ctx, "app.example.com"); found {
		t.Error("a non-root token stored a row")
	}
}

func TestDomainAuthGateRoutes_RequireAuth(t *testing.T) {
	rt, db := newTestRouter(t)
	seedAppWithDomain(t, db)

	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, authGatePath},
		{http.MethodPut, authGatePath},
		{http.MethodDelete, authGatePath},
	})
}
