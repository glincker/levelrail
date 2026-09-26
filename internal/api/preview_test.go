package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/preview"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakePreview struct {
	dir      string
	records  map[string]preview.Record
	settings map[string]preview.AppSettings
	captured []string
	deleted  []string
}

func newFakePreview(t *testing.T) *fakePreview {
	t.Helper()
	return &fakePreview{dir: t.TempDir(), records: map[string]preview.Record{}, settings: map[string]preview.AppSettings{}}
}

func (f *fakePreview) Config() preview.Config {
	return preview.Config{Enabled: true, KeepPerApp: 5, TTL: 30 * 24 * time.Hour, MaxTotalBytes: 200 << 20}
}

func (f *fakePreview) Settings(_ context.Context, app string) (preview.AppSettings, error) {
	s, ok := f.settings[app]
	if !ok {
		s = preview.AppSettings{App: app, Path: "/"}
	}
	return s, nil
}

func (f *fakePreview) SaveSettings(ctx context.Context, app string, p preview.SettingsPatch) (preview.AppSettings, error) {
	s, _ := f.Settings(ctx, app)
	if p.Enabled != nil {
		s.Enabled = *p.Enabled
	}
	if p.Path != nil {
		s.Path = *p.Path
	}
	if p.WaitMS != nil {
		s.WaitMS = *p.WaitMS
	}
	if err := s.Validate(); err != nil {
		return preview.AppSettings{}, err
	}
	f.settings[app] = s
	return s, nil
}

func (f *fakePreview) List(_ context.Context, app string) ([]preview.Record, error) {
	var out []preview.Record
	for _, r := range f.records {
		if r.App == app {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakePreview) OpenImage(_ context.Context, app, id string) (string, preview.Record, error) {
	r, ok := f.records[id]
	if !ok || r.App != app || r.Status != preview.StatusOK {
		return "", preview.Record{}, preview.ErrNotFound
	}
	return filepath.Join(f.dir, id+".jpg"), r, nil
}

func (f *fakePreview) Capture(_ context.Context, app string) error {
	if !f.settings[app].Enabled {
		return preview.ErrDisabled
	}
	f.captured = append(f.captured, app)
	return nil
}

func (f *fakePreview) Prune(context.Context, string, bool) (preview.PruneResult, error) {
	return preview.PruneResult{Removed: 2, FreedBytes: 4096}, nil
}

func (f *fakePreview) Status(ctx context.Context, app string) (preview.Summary, error) {
	s, _ := f.Settings(ctx, app)
	recs, _ := f.List(ctx, app)
	sum := preview.Summary{Settings: s, GlobalOn: true, Image: "browser:1"}
	if len(recs) > 0 {
		sum.Latest = &recs[0]
	}
	return sum, nil
}

func (f *fakePreview) DeleteApp(_ context.Context, app string) error {
	f.deleted = append(f.deleted, app)
	return nil
}

func (f *fakePreview) addOK(t *testing.T, app, id string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, id+".jpg"), []byte("\xff\xd8jpegdata"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.records[id] = preview.Record{DeploymentID: id, App: app, Path: "/", Status: preview.StatusOK, Bytes: 10, CapturedAt: time.Unix(1700000000, 0)}
}

func newPreviewTestRouter(t *testing.T) (*Router, *store.DB, *fakePreview) {
	t.Helper()
	rt, db, _ := newPipelineRouter(t)
	fp := newFakePreview(t)
	rt.SetPreview(fp)
	return rt, db, fp
}

func TestPreviewRoutes_RequireAuth(t *testing.T) {
	rt, _, _ := newPreviewTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/apps/web/preview"},
		{http.MethodPut, "/api/v1/apps/web/preview"},
		{http.MethodPost, "/api/v1/apps/web/preview/capture"},
		{http.MethodPost, "/api/v1/apps/web/preview/prune"},
		{http.MethodGet, "/api/v1/apps/web/preview/history"},
		{http.MethodGet, "/api/v1/apps/web/deployments/dep_a/preview"},
	})
}

func TestPreviewRoutes_ReadOnlyTokenCannotMutate(t *testing.T) {
	rt, db, _ := newPreviewTestRouter(t)
	assertProviderRoutesForbiddenForAbilities(t, rt, db, "tok", "plain-secret", []string{AbilityRead}, []providerRouteCase{
		{method: http.MethodPut, path: "/api/v1/apps/web/preview", body: `{"enabled":true}`},
		{method: http.MethodPost, path: "/api/v1/apps/web/preview/capture"},
		{method: http.MethodPost, path: "/api/v1/apps/web/preview/prune", body: `{}`},
	})
}

func TestPreviewImage_IAMDenyScopedToApp(t *testing.T) {
	rt, db, fp := newPreviewTestRouter(t)
	bootstrapTestAdmin(t, db)
	seedScheduledTaskApp(t, db, "secret-app")
	seedScheduledTaskApp(t, db, "open-app")
	fp.addOK(t, "secret-app", "dep_secret")
	fp.addOK(t, "open-app", "dep_open")
	reader := storeUserWithAbilitiesForTest(t, db, "preview-reader@example.com", []string{AbilityRead})
	cookie := sessionCookieForTest(t, rt, reader.ID)
	attachTestPolicy(t, db, "deny-secret-preview", "Deny", AbilityRead, "app:secret-app", store.PrincipalTypeUser, reader.ID)

	do := func(target string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, target, ""))
		return rec
	}

	if rec := do("/api/v1/apps/secret-app/deployments/dep_secret/preview"); rec.Code != http.StatusForbidden {
		t.Errorf("denied app image status = %d, want 403", rec.Code)
	}
	rec := do("/api/v1/apps/open-app/deployments/dep_open/preview")
	if rec.Code != http.StatusOK {
		t.Fatalf("open app image status = %d, want 200, body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Errorf("content type = %q, want image/jpeg", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "private") {
		t.Errorf("cache control = %q, want private", cc)
	}
	if rec := do("/api/v1/apps/open-app/deployments/dep_secret/preview"); rec.Code != http.StatusNotFound {
		t.Errorf("cross-app deployment status = %d, want 404", rec.Code)
	}
	if rec := do("/api/v1/apps/open-app/deployments/dep_missing/preview"); rec.Code != http.StatusNotFound {
		t.Errorf("missing deployment status = %d, want 404", rec.Code)
	}
}

func TestPreviewSettings_RoundTripAndValidation(t *testing.T) {
	rt, db, fp := newPreviewTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")

	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/apps/web/preview", body))
		return rec
	}
	rec := put(`{"enabled":true,"path":"/pricing","wait_ms":500}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status = %d, body %s", rec.Code, rec.Body.String())
	}
	var got previewStatusResource
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Enabled || got.Path != "/pricing" || got.WaitMS != 500 || !got.ServerEnabled {
		t.Errorf("status = %+v", got)
	}
	for _, bad := range []string{`{"path":"//evil.example"}`, `{"path":"http://evil.example/"}`, `{"path":"relative"}`, `{"wait_ms":999999}`} {
		if rec := put(bad); rec.Code != http.StatusBadRequest {
			t.Errorf("put %s status = %d, want 400", bad, rec.Code)
		}
	}
	if fp.settings["web"].Path != "/pricing" {
		t.Errorf("invalid puts changed stored path to %q", fp.settings["web"].Path)
	}
}

func TestPreviewCapture_DisabledAndAccepted(t *testing.T) {
	rt, db, fp := newPreviewTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	post := func() int {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/apps/web/preview/capture", ""))
		return rec.Code
	}
	if got := post(); got != http.StatusConflict {
		t.Errorf("capture while off = %d, want 409", got)
	}
	fp.settings["web"] = preview.AppSettings{App: "web", Enabled: true, Path: "/"}
	if got := post(); got != http.StatusAccepted {
		t.Errorf("capture while on = %d, want 202", got)
	}
	if len(fp.captured) != 1 {
		t.Errorf("captured = %v, want one", fp.captured)
	}
}

func TestPreviewUnconfigured_Returns501(t *testing.T) {
	rt, db, _ := newPipelineRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/preview", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}

func TestDeployAttempts_IncludePreviewImageURL(t *testing.T) {
	rt, db, fp := newPreviewTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedScheduledTaskApp(t, db, "web")
	for _, id := range []string{"dep_with", "dep_without"} {
		if err := db.SaveDeployAttempt(context.Background(), store.DeployAttempt{
			ID: id, ServiceName: "web", Image: "levelrail/web:1", Source: store.DeployAttemptSourceImage,
			Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now(),
		}); err != nil {
			t.Fatalf("seed attempt: %v", err)
		}
	}
	fp.addOK(t, "web", "dep_with")

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/apps/web/deploy-attempts", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var items []deployAttemptResource
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	urls := map[string]string{}
	for _, it := range items {
		urls[it.ID] = it.PreviewImageURL
	}
	want := "/api/v1/apps/web/deployments/dep_with/preview?v=1700000000"
	if urls["dep_with"] != want {
		t.Errorf("dep_with preview url = %q, want %q", urls["dep_with"], want)
	}
	if urls["dep_without"] != "" {
		t.Errorf("dep_without preview url = %q, want empty", urls["dep_without"])
	}
}
