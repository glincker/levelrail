package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleUpdateApp_RegistryCredentialSetKeepClearAndEcho(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	for _, c := range []store.RegistryCredential{
		{ID: "rc-a", Name: "a-pull", RegistryHost: "registry.example.com", Username: "u"},
		{ID: "rc-b", Name: "b-pull", RegistryHost: "registry.example.com", Username: "u"},
	} {
		if err := db.SaveRegistryCredential(ctx, c); err != nil {
			t.Fatalf("seed credential: %v", err)
		}
	}
	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web", body))
		return rec
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps",
		`{"name":"web","image":"registry.example.com/team/web:1","port":80,"registry_credential":"a-pull"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %s", rec.Code, rec.Body.String())
	}
	stored := func() string {
		svc, err := db.GetDesiredService(ctx, "web")
		if err != nil {
			t.Fatal(err)
		}
		return svc.RegistryCredentialID
	}

	if rec := put(`{"replicas":2}`); rec.Code != http.StatusOK || stored() != "rc-a" {
		t.Fatalf("an unsent field must keep the credential: %d %s %q", rec.Code, rec.Body.String(), stored())
	}
	if rec := put(`{"registry_credential":"b-pull"}`); rec.Code != http.StatusOK || stored() != "rc-b" {
		t.Fatalf("replace: %d %s %q", rec.Code, rec.Body.String(), stored())
	}
	if rec := put(`{"registry_credential":"nope"}`); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "registry_credential_not_found") {
		t.Fatalf("unknown name: %d %s", rec.Code, rec.Body.String())
	}
	get := httptest.NewRecorder()
	rt.Handler().ServeHTTP(get, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web", ""))
	if !strings.Contains(get.Body.String(), `"registry_credential":"b-pull"`) {
		t.Fatalf("read does not echo the name: %s", get.Body.String())
	}
	if rec := put(`{"registry_credential":null}`); rec.Code != http.StatusOK || stored() != "" {
		t.Fatalf("clear: %d %s %q", rec.Code, rec.Body.String(), stored())
	}
}
