package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/docker"
)

type fakeCompatVolumes struct {
	vols  []docker.NamedVolume
	files map[string]string
}

func (f *fakeCompatVolumes) ListNamedVolumes(context.Context) ([]docker.NamedVolume, error) {
	return f.vols, nil
}
func (f *fakeCompatVolumes) RemoveVolume(context.Context, string) error { return nil }
func (f *fakeCompatVolumes) ReadVolumeFile(_ context.Context, vol, rel string) ([]byte, error) {
	if s, ok := f.files[vol+"/"+rel]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("not found")
}

func TestLeftoverVolumeIncompatibleCreateRefused(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.orphanedVolumes = &fakeCompatVolumes{
		vols:  []docker.NamedVolume{{Name: "db-pg-data"}},
		files: map[string]string{"db-pg-data/PG_VERSION": "18\n"},
	}
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"pg","engine":"postgres","version":"19.0"}`))
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "leftover_volume_incompatible") {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{"18", "19.0", "different name", "discard"} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("message lacks %q: %s", want, rec.Body.String())
		}
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"pg","engine":"postgres","version":"19.0","existing_volume":"discard"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("discard create status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLeftoverVolumeLatestPicksVolumeMajor(t *testing.T) {
	rt, db := newTestRouter(t)
	rt.orphanedVolumes = &fakeCompatVolumes{
		vols:  []docker.NamedVolume{{Name: "db-pg-data"}},
		files: map[string]string{"db-pg-data/PG_VERSION": "17"},
	}
	rt.imageTagLister = &fakeLatestLister{tags: []string{"17.2", "17.7", "18.1"}}
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases", `{"name":"pg","engine":"postgres"}`))
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := getDatabaseResource(t, rt, cookie, "pg"); got.Version != "17.7" {
		t.Errorf("version = %q, want 17.7", got.Version)
	}
}
