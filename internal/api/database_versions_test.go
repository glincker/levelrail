package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbcatalogbuild"
	"github.com/GLINCKER/levelrail/internal/dbupgrade"
)

type fakeVersionCatalog struct {
	cooled  bool
	liveErr error
	enabled *bool
}

func (f *fakeVersionCatalog) Engines() []string { return []string{"postgres"} }
func (f *fakeVersionCatalog) Versions(engine string) (dbupgrade.VersionsView, error) {
	if engine != "postgres" {
		return dbupgrade.VersionsView{}, dbupgrade.ErrUnknownEngine
	}
	return dbupgrade.VersionsView{Engine: engine, Origin: "embedded", Freshness: "ok",
		Versions: []dbupgrade.VersionInfo{{Version: "17.7", Channel: "stable", Recommended: true}}}, nil
}
func (f *fakeVersionCatalog) LiveRefresh(_ context.Context, engine string) (dbupgrade.VersionsView, bool, error) {
	if f.liveErr != nil {
		return dbupgrade.VersionsView{}, false, f.liveErr
	}
	v, _ := f.Versions(engine)
	v.Origin = "live"
	return v, f.cooled, nil
}
func (f *fakeVersionCatalog) Status() dbupgrade.Status      { return dbupgrade.Status{Source: "embedded"} }
func (f *fakeVersionCatalog) Refresh(context.Context) error { return nil }
func (f *fakeVersionCatalog) SetRemoteEnabled(b bool) error { f.enabled = &b; return nil }

func TestDatabaseVersionRoutes(t *testing.T) {
	rt, _, cookie := newUpgradeTestRouter(t)
	fake := &fakeVersionCatalog{}
	rt.SetDatabaseVersionCatalog(fake)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}
	if rec := do(http.MethodGet, "/api/v1/database-versions?engine=postgres", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"recommended":true`) {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(http.MethodGet, "/api/v1/database-versions?engine=nope", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown engine: %d", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/database-versions/refresh", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing engine: %d", rec.Code)
	}
	fake.cooled = true
	if rec := do(http.MethodPost, "/api/v1/database-versions/refresh?engine=postgres", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "checked just now") {
		t.Fatalf("cooldown: %d %s", rec.Code, rec.Body.String())
	}
	fake.liveErr = &dbcatalogbuild.RateLimitedError{Source: "registry", RetryAfter: 30 * time.Second}
	rec := do(http.MethodPost, "/api/v1/database-versions/refresh?engine=postgres", "")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "31" {
		t.Fatalf("429: %d retry-after %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	if rec := do(http.MethodPut, "/api/v1/settings/database-catalog", `{"remote_enabled":false}`); rec.Code != http.StatusOK || fake.enabled == nil || *fake.enabled {
		t.Fatalf("setting: %d", rec.Code)
	}
}
