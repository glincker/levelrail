package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/store"
)

func TestHandleCreateApp_RegistryCredentialByName(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	cred := store.RegistryCredential{ID: "rc-gitlab", Name: "gitlab", RegistryHost: "registry.example.com", Username: "u"}
	if err := db.SaveRegistryCredential(ctx, cred); err != nil {
		t.Fatalf("seed credential: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps",
		`{"name":"web","image":"registry.example.com/team/web:1","port":80,"registry_credential":"gitlab"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d body = %s", rec.Code, rec.Body.String())
	}
	svc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if svc.RegistryCredentialID != cred.ID {
		t.Fatalf("RegistryCredentialID = %q, want %q", svc.RegistryCredentialID, cred.ID)
	}

	// A later deploy by image must keep pulling with the stored credential.
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/deploys",
		`{"image":"registry.example.com/team/web:2"}`))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("deploy status = %d body = %s", rec.Code, rec.Body.String())
	}
	svc, err = db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	if svc.RegistryCredentialID != cred.ID || svc.Image != "registry.example.com/team/web:2" {
		t.Fatalf("after deploy: credential %q image %q", svc.RegistryCredentialID, svc.Image)
	}
}

func TestHandleCreateApp_UnknownRegistryCredentialFails(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps",
		`{"name":"web","image":"registry.example.com/team/web:1","port":80,"registry_credential":"nope"}`))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "registry_credential_not_found") {
		t.Fatalf("status = %d body = %s, want 400 registry_credential_not_found", rec.Code, rec.Body.String())
	}
	if _, err := db.GetDesiredService(context.Background(), "web"); err == nil {
		t.Fatal("app must not be created with an unknown credential")
	}
}
