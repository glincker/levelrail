package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/registrytoken"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakePresetRefresher struct {
	minted     registrytoken.Minted
	err        error
	refreshErr error
	refreshed  []string
	db         *store.DB
}

func (f *fakePresetRefresher) Preview(context.Context, string, registrytoken.Config) (registrytoken.Minted, error) {
	return f.minted, f.err
}

func (f *fakePresetRefresher) Refresh(ctx context.Context, id string) (store.RegistryCredential, error) {
	f.refreshed = append(f.refreshed, id)
	if f.refreshErr != nil {
		return store.RegistryCredential{}, f.refreshErr
	}
	return f.db.GetRegistryCredential(ctx, id)
}

func (f *fakePresetRefresher) NextRefresh(_, expires time.Time) time.Time {
	return expires.Add(-time.Hour)
}
func (f *fakePresetRefresher) Tick(context.Context) error { return nil }
func (f *fakePresetRefresher) Interval() time.Duration    { return time.Minute }

func newPresetRouter(t *testing.T, ref *fakePresetRefresher, setter *fakeRegistryCredentialSecretsSetter) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	ref.db = db
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	return NewRouter(logger, testBrand(), db, WithRegistryCredentialSecrets(setter), WithRegistryRefresher(ref)), db
}

func TestCreatePresetRegistryCredential(t *testing.T) {
	exp := time.Now().Add(12 * time.Hour).UTC()
	ref := &fakePresetRefresher{minted: registrytoken.Minted{RegistryHost: "1.dkr.ecr.us-east-1.amazonaws.com", Username: "AWS", Password: "minted-secret", ExpiresAt: exp}}
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newPresetRouter(t, ref, setter)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", `{"name":"ecr","preset":"ecr","region":"us-east-1"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "minted-secret") {
		t.Fatalf("password leaked in response: %s", rec.Body.String())
	}
	if len(setter.calls) != 1 || setter.calls[0].plaintext != "minted-secret" {
		t.Fatalf("secret writes = %+v", setter.calls)
	}
	list, err := db.ListRegistryCredentials(context.Background())
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %v, %v", list, err)
	}
	c := list[0]
	if c.Preset != "ecr" || c.Username != "AWS" || c.NextRefreshAt == nil || c.LastRefreshedAt == nil || c.ExpiresAt == nil || !strings.Contains(c.PresetConfig, "us-east-1") {
		t.Fatalf("stored = %+v", c)
	}
}

func TestPresetTestAndMissingConnection(t *testing.T) {
	ref := &fakePresetRefresher{err: registrytoken.ErrNoCloudConnection}
	rt, db := newPresetRouter(t, ref, &fakeRegistryCredentialSecretsSetter{})
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/presets/test", `{"preset":"acr"}`))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("acr without host: status = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/presets/test", `{"preset":"artifact_registry"}`))
	if rec.Code != http.StatusPreconditionFailed {
		t.Fatalf("no connection: status = %d body = %s", rec.Code, rec.Body.String())
	}

	ref.err = nil
	ref.minted = registrytoken.Minted{RegistryHost: "us-docker.pkg.dev", Username: "oauth2accesstoken", Password: "tok", ExpiresAt: time.Now().Add(time.Hour)}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/presets/test", `{"preset":"artifact_registry"}`))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "tok\"") {
		t.Fatalf("test: status = %d body = %s", rec.Code, rec.Body.String())
	}
}

func TestRefreshRegistryCredentialFailureIsReported(t *testing.T) {
	ref := &fakePresetRefresher{minted: registrytoken.Minted{RegistryHost: "h", Username: "u", Password: "p", ExpiresAt: time.Now().Add(time.Hour)}}
	rt, db := newPresetRouter(t, ref, &fakeRegistryCredentialSecretsSetter{})
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", `{"name":"gar","preset":"artifact_registry"}`))
	var created registryCredentialResource
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d", rec.Code)
	}
	list, _ := db.ListRegistryCredentials(context.Background())
	created.ID = list[0].ID

	ref.refreshErr = errors.New("registrytoken: access denied")
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/"+created.ID+"/refresh", ``))
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "access denied") {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	ref.refreshErr = nil
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials/"+created.ID+"/refresh", ``))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
}
