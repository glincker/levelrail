package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleStopProject_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/proj_nope/stop", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

func TestHandleStartProject_NotFound(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/proj_nope/start", ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}

// TestHandleStopProject_SuspendsEveryAppAndDatabase and its start-side
// counterpart exercise the real store end to end: every app and database
// filed under the project gets suspended/resumed, nothing outside the
// project is touched.
func TestHandleStopProject_SuspendsEveryAppAndDatabase(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "demo"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	seedStopProjectFixtures(ctx, t, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/proj_1/stop", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got projectLifecycleResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.SucceededApps) != 2 || got.SucceededApps[0] != "api" || got.SucceededApps[1] != "web" {
		t.Fatalf("SucceededApps = %v, want [api web]", got.SucceededApps)
	}
	if len(got.SucceededDatabases) != 1 || got.SucceededDatabases[0] != "main" {
		t.Fatalf("SucceededDatabases = %v, want [main]", got.SucceededDatabases)
	}
	if len(got.FailedApps) != 0 || len(got.FailedDatabases) != 0 {
		t.Fatalf("FailedApps/FailedDatabases = %v/%v, want none", got.FailedApps, got.FailedDatabases)
	}

	webSvc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService(web) error = %v", err)
	}
	if !webSvc.Suspended {
		t.Error("web.Suspended = false, want true")
	}
	otherSvc, err := db.GetDesiredService(ctx, "other")
	if err != nil {
		t.Fatalf("GetDesiredService(other) error = %v", err)
	}
	if otherSvc.Suspended {
		t.Error("other.Suspended = true, want false (not in this project)")
	}
	mainDB, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase(main) error = %v", err)
	}
	if !mainDB.Suspended {
		t.Error("main.Suspended = false, want true")
	}
}

func TestHandleStartProject_ResumesEveryAppAndDatabase(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()

	if err := db.SaveProject(ctx, store.Project{ID: "proj_1", Name: "demo"}); err != nil {
		t.Fatalf("SaveProject() error = %v", err)
	}
	seedStopProjectFixtures(ctx, t, db)
	if err := db.UpdateServiceSuspended(ctx, "web", true); err != nil {
		t.Fatalf("seed suspend web: %v", err)
	}
	if err := db.UpdateServiceSuspended(ctx, "api", true); err != nil {
		t.Fatalf("seed suspend api: %v", err)
	}
	if err := db.UpdateDatabaseSuspended(ctx, "main", true); err != nil {
		t.Fatalf("seed suspend main: %v", err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/projects/proj_1/start", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	webSvc, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatalf("GetDesiredService(web) error = %v", err)
	}
	if webSvc.Suspended {
		t.Error("web.Suspended = true, want false")
	}
	mainDB, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil {
		t.Fatalf("GetDesiredDatabase(main) error = %v", err)
	}
	if mainDB.Suspended {
		t.Error("main.Suspended = true, want false")
	}
}

// seedStopProjectFixtures saves two apps and one database under proj_1
// plus one app left project-less, so tests can assert the project scope
// is respected, not just that suspend/resume works at all.
func seedStopProjectFixtures(ctx context.Context, t *testing.T, db *store.DB) {
	t.Helper()
	for _, svc := range []store.DesiredService{
		{Name: "web", Image: "img:v1", Port: 8080},
		{Name: "api", Image: "img:v1", Port: 8081},
		{Name: "other", Image: "img:v1", Port: 8082},
	} {
		if err := db.SaveDesiredService(ctx, svc); err != nil {
			t.Fatalf("SaveDesiredService(%s) error = %v", svc.Name, err)
		}
	}
	if err := db.UpdateServiceProject(ctx, "web", "proj_1"); err != nil {
		t.Fatalf("UpdateServiceProject(web) error = %v", err)
	}
	if err := db.UpdateServiceProject(ctx, "api", "proj_1"); err != nil {
		t.Fatalf("UpdateServiceProject(api) error = %v", err)
	}
	if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}); err != nil {
		t.Fatalf("SaveDesiredDatabase(main) error = %v", err)
	}
	if err := db.UpdateDatabaseProject(ctx, "main", "proj_1"); err != nil {
		t.Fatalf("UpdateDatabaseProject(main) error = %v", err)
	}
}

// fakeProjectLifecycleAppStore/fakeProjectLifecycleDatabaseStore/
// fakeProjectLifecycleProjectStore are hand-written fakes, the same
// "not a mocking framework" convention fakeDrainAppStore (nodes_test.go)
// already establishes, used only by
// TestHandleStopProject_PartialFailure: a real *store.DB has no way to
// make UpdateServiceSuspended fail for one specific service on demand.
type fakeProjectLifecycleAppStore struct {
	services    []store.DesiredService
	failOnName  string
	failErr     error
	updateCalls []string
}

func (f *fakeProjectLifecycleAppStore) SaveDesiredService(context.Context, store.DesiredService) error {
	return nil
}
func (f *fakeProjectLifecycleAppStore) GetDesiredService(context.Context, string) (*store.DesiredService, error) {
	return nil, store.ErrServiceNotFound
}
func (f *fakeProjectLifecycleAppStore) ListDesiredServices(context.Context) ([]store.DesiredService, error) {
	return nil, nil
}
func (f *fakeProjectLifecycleAppStore) DeleteDesiredService(context.Context, string) error {
	return nil
}
func (f *fakeProjectLifecycleAppStore) UpdateServiceNode(context.Context, string, string) error {
	return nil
}
func (f *fakeProjectLifecycleAppStore) ListDesiredServicesByNode(context.Context, string) ([]store.DesiredService, error) {
	return nil, nil
}
func (f *fakeProjectLifecycleAppStore) ListDesiredServicesByProject(context.Context, string) ([]store.DesiredService, error) {
	return f.services, nil
}
func (f *fakeProjectLifecycleAppStore) RestartService(context.Context, string) error { return nil }
func (f *fakeProjectLifecycleAppStore) UpdateServiceProject(context.Context, string, string) error {
	return nil
}
func (f *fakeProjectLifecycleAppStore) UpdateServiceStorageTarget(context.Context, string, string) error {
	return nil
}
func (f *fakeProjectLifecycleAppStore) UpdateServiceSuspended(_ context.Context, name string, _ bool) error {
	f.updateCalls = append(f.updateCalls, name)
	if name == f.failOnName {
		return f.failErr
	}
	return nil
}
func (f *fakeProjectLifecycleAppStore) UpdateServiceApp(context.Context, string, string) error {
	return nil
}
func (f *fakeProjectLifecycleAppStore) UpdateServiceLogDrain(context.Context, string, *store.LogDrain) error {
	return nil
}
func (f *fakeProjectLifecycleAppStore) UpdateServiceDatabaseAttachment(context.Context, string, *store.DatabaseAttachment) error {
	return nil
}

type fakeProjectLifecycleDatabaseStore struct {
	databases   []store.DesiredDatabase
	failOnName  string
	failErr     error
	updateCalls []string
}

func (fakeProjectLifecycleDatabaseStore) SaveDesiredDatabase(context.Context, store.DesiredDatabase) error {
	return nil
}
func (fakeProjectLifecycleDatabaseStore) GetDesiredDatabase(context.Context, string) (*store.DesiredDatabase, error) {
	return nil, store.ErrDatabaseNotFound
}
func (fakeProjectLifecycleDatabaseStore) ListDesiredDatabases(context.Context) ([]store.DesiredDatabase, error) {
	return nil, nil
}
func (fakeProjectLifecycleDatabaseStore) DeleteDesiredDatabase(context.Context, string) error {
	return nil
}
func (fakeProjectLifecycleDatabaseStore) UpdateDatabaseNode(context.Context, string, string) error {
	return nil
}
func (f *fakeProjectLifecycleDatabaseStore) ListDesiredDatabasesByNode(context.Context, string) ([]store.DesiredDatabase, error) {
	return nil, nil
}
func (f *fakeProjectLifecycleDatabaseStore) ListDesiredDatabasesByProject(context.Context, string) ([]store.DesiredDatabase, error) {
	return f.databases, nil
}
func (fakeProjectLifecycleDatabaseStore) UpdateDatabaseProject(context.Context, string, string) error {
	return nil
}
func (f *fakeProjectLifecycleDatabaseStore) UpdateDatabaseSuspended(_ context.Context, name string, _ bool) error {
	f.updateCalls = append(f.updateCalls, name)
	if name == f.failOnName {
		return f.failErr
	}
	return nil
}
func (fakeProjectLifecycleDatabaseStore) SetDatabaseBackupSchedule(context.Context, string, string, string, int, int) error {
	return nil
}
func (fakeProjectLifecycleDatabaseStore) SetDatabasePublicAccess(context.Context, string, bool, int) (int, error) {
	return 0, nil
}

type fakeProjectLifecycleProjectStore struct {
	project *store.Project
}

func (f *fakeProjectLifecycleProjectStore) SaveProject(context.Context, store.Project) error {
	return nil
}
func (f *fakeProjectLifecycleProjectStore) GetProject(_ context.Context, id string) (store.Project, error) {
	if f.project != nil && f.project.ID == id {
		return *f.project, nil
	}
	return store.Project{}, store.ErrProjectNotFound
}
func (f *fakeProjectLifecycleProjectStore) ListProjects(context.Context) ([]store.Project, error) {
	return nil, nil
}
func (f *fakeProjectLifecycleProjectStore) DeleteProject(context.Context, string) error { return nil }
func (f *fakeProjectLifecycleProjectStore) SetProjectEnvVars(context.Context, string, map[string]string) error {
	return nil
}
func (f *fakeProjectLifecycleProjectStore) ListProjectEnvVars(context.Context, string) (map[string]string, error) {
	return nil, nil
}

// TestHandleStopProject_PartialFailure: one app and one database out of
// several fail to suspend. The handler must keep going, still attempt
// every remaining resource, and report the failure per resource type
// rather than aborting or silently reporting full success.
func TestHandleStopProject_PartialFailure(t *testing.T) {
	apps := &fakeProjectLifecycleAppStore{
		services: []store.DesiredService{
			{Name: "api"}, {Name: "web"}, {Name: "worker"},
		},
		failOnName: "web",
		failErr:    errors.New("write conflict"),
	}
	databases := &fakeProjectLifecycleDatabaseStore{
		databases: []store.DesiredDatabase{
			{Name: "cache"}, {Name: "main"},
		},
		failOnName: "main",
		failErr:    errors.New("write conflict"),
	}
	projects := &fakeProjectLifecycleProjectStore{project: &store.Project{ID: "proj_1", Name: "demo"}}

	rt := &Router{
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		apps:      apps,
		databases: databases,
		projects:  projects,
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj_1/stop", nil)
	req.SetPathValue("id", "proj_1")
	rec := httptest.NewRecorder()
	rt.handleStopProject(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got projectLifecycleResult
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.SucceededApps) != 2 || got.SucceededApps[0] != "api" || got.SucceededApps[1] != "worker" {
		t.Fatalf("SucceededApps = %v, want [api worker] (must keep going past the failure)", got.SucceededApps)
	}
	if len(got.FailedApps) != 1 || got.FailedApps[0] != "web" {
		t.Fatalf("FailedApps = %v, want [web]", got.FailedApps)
	}
	if len(got.SucceededDatabases) != 1 || got.SucceededDatabases[0] != "cache" {
		t.Fatalf("SucceededDatabases = %v, want [cache]", got.SucceededDatabases)
	}
	if len(got.FailedDatabases) != 1 || got.FailedDatabases[0] != "main" {
		t.Fatalf("FailedDatabases = %v, want [main]", got.FailedDatabases)
	}
	if len(apps.updateCalls) != 3 {
		t.Fatalf("UpdateServiceSuspended call count = %d, want 3: every listed app must be attempted", len(apps.updateCalls))
	}
	if len(databases.updateCalls) != 2 {
		t.Fatalf("UpdateDatabaseSuspended call count = %d, want 2: every listed database must be attempted", len(databases.updateCalls))
	}
}
