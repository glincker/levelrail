package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

type fakeLatestLister struct {
	tags  []string
	err   error
	calls int
	refs  []string
}

func (f *fakeLatestLister) ListTags(_ context.Context, ref string, _ *docker.RegistryAuth) ([]string, error) {
	f.calls++
	f.refs = append(f.refs, ref)
	return f.tags, f.err
}

func TestLatestDatabaseVersion_OmittedResolvesAndStoresConcrete(t *testing.T) {
	rt, db := newTestRouter(t)
	lister := &fakeLatestLister{tags: []string{"16.2", "17.7", "17", "18.1", "18.1-alpine", "19.0", "18rc1", "latest", "19-beta1"}}
	rt.imageTagLister = lister
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"main","engine":"postgres"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var got databaseResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != "18.1" || got.VersionResolution == nil || got.VersionResolution.Source != "registry" {
		t.Fatalf("response = %+v", got)
	}
	if got.VersionResolution.SkippedNewerMajor != "19.0" {
		t.Errorf("skipped = %q, want 19.0", got.VersionResolution.SkippedNewerMajor)
	}
	if stored := getDatabaseResource(t, rt, cookie, "main"); stored.Version != "18.1" {
		t.Errorf("stored version = %q, want 18.1", stored.Version)
	}
	if len(lister.refs) != 1 || lister.refs[0] != "postgres" {
		t.Errorf("refs = %v, want only the engine's own repository", lister.refs)
	}
}

func TestLatestDatabaseVersion_KeywordAndCache(t *testing.T) {
	rt, db := newTestRouter(t)
	lister := &fakeLatestLister{tags: []string{"7.4.6", "8.2.2", "8.2"}}
	rt.imageTagLister = lister
	cookie := loginTestSession(t, rt, db)
	for _, n := range []string{"a", "b"} {
		mustCreateDatabase(t, rt, cookie, `{"name":"`+n+`","engine":"redis","version":"latest"}`)
	}
	if got := getDatabaseResource(t, rt, cookie, "b").Version; got != "8.2.2" {
		t.Errorf("version = %q, want 8.2.2", got)
	}
	if lister.calls != 1 {
		t.Errorf("registry calls = %d, want 1 (cached)", lister.calls)
	}
}

func TestLatestDatabaseVersion_RegistryOutageFallsBackToEngineDefault(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.imageTagLister = &fakeLatestLister{err: errors.New("down")}
	cookie := loginTestSession(t, rt, db)
	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"postgres"}`)
	if got := getDatabaseResource(t, rt, cookie, "main").Version; got != defaultEngineVersion("postgres") {
		t.Errorf("version = %q, want engine default", got)
	}
}

func TestLatestDatabaseVersion_ExplicitVersionUntouched(t *testing.T) {
	rt, db := newTestRouter(t)
	lister := &fakeLatestLister{tags: []string{"18.1"}}
	rt.imageTagLister = lister
	cookie := loginTestSession(t, rt, db)
	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"postgres","version":"16"}`)
	if got := getDatabaseResource(t, rt, cookie, "main").Version; got != "16" || lister.calls != 0 {
		t.Errorf("version = %q calls = %d", got, lister.calls)
	}
}

func TestLatestDatabaseVersion_SettingOffRequiresVersion(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.imageTagLister = &fakeLatestLister{tags: []string{"18.1"}}
	cookie := loginTestSession(t, rt, db)
	if _, err := rt.RuntimeSettings().Set(context.Background(), runtimesettings.KeyDatabasesAutoLatest, json.RawMessage("false"), "test"); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"name":"main","engine":"postgres"}`, `{"name":"main","engine":"postgres","version":"latest"}`} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", body))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", body, rec.Code)
		}
	}
}

func TestLatestDatabaseVersion_InvalidNameSkipsRegistryLookup(t *testing.T) {
	rt, db := newTestRouter(t)
	lister := &fakeLatestLister{tags: []string{"18.1"}}
	rt.imageTagLister = lister
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"Bad Name","engine":"postgres"}`))
	if rec.Code != http.StatusBadRequest || lister.calls != 0 {
		t.Fatalf("status = %d, registry calls = %d", rec.Code, lister.calls)
	}
}
