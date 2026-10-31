package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestHandleEnablePITR_RejectsAgentDirectoryTarget(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedPostgresDatabaseForAPI(t, db)
	ctx := context.Background()
	target := store.BackupTarget{ID: "bkt_agent", Name: "node dir", Provider: store.BackupProviderCustom, Endpoint: store.AgentEndpointScheme + "node-1", Bucket: "/srv/backups", CreatedAt: "2026-08-14T00:00:00Z"}
	if err := db.SaveBackupTarget(ctx, target); err != nil {
		t.Fatal(err)
	}
	if err := db.SetDatabaseBackupSchedule(ctx, "main", target.ID, "0 3 * * *", 7, 0); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/pitr", ``))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
	}
	got, err := db.GetDesiredDatabase(ctx, "main")
	if err != nil || got.PITREnabled {
		t.Fatalf("PITR must stay off: enabled=%v err=%v", got != nil && got.PITREnabled, err)
	}
}

type flakyTeardownRuntime struct {
	*fakeExecAppRuntime
	removeErr   error
	removeCalls int
}

func (f *flakyTeardownRuntime) InspectByName(context.Context, string) (*docker.ContainerState, error) {
	return &docker.ContainerState{ID: "c1", Running: false}, nil
}

func (f *flakyTeardownRuntime) Remove(context.Context, string, bool) error {
	f.removeCalls++
	return f.removeErr
}

func TestDatabaseTeardownHalfSucceededIsRetried(t *testing.T) {
	t.Parallel()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db)
	fake := &flakyTeardownRuntime{fakeExecAppRuntime: &fakeExecAppRuntime{}, removeErr: errors.New("docker unavailable")}
	rt.execRuntime = func(string) (docker.Runtime, error) { return fake, nil }
	ctx := context.Background()

	if err := rt.recordDatabaseTeardown(ctx, "main", "", false); err != nil {
		t.Fatal(err)
	}
	if err := rt.runDatabaseTeardown(ctx, "main", "", false); err == nil {
		t.Fatal("teardown must report the runtime failure")
	}
	jobs, _ := db.ListDatabaseTeardownJobs(ctx)
	if len(jobs) != 1 || jobs[0].Attempts != 1 || !strings.Contains(jobs[0].LastError, "docker unavailable") {
		t.Fatalf("failed teardown must stay pending with its error: %+v", jobs)
	}

	if err := rt.RetryDatabaseTeardowns(ctx); err == nil {
		t.Fatal("retry must still report the failure while docker is down")
	}

	fake.removeErr = nil
	if err := rt.RetryDatabaseTeardowns(ctx); err != nil {
		t.Fatalf("retry after recovery: %v", err)
	}
	if jobs, _ = db.ListDatabaseTeardownJobs(ctx); len(jobs) != 0 {
		t.Fatalf("job must be cleared once the teardown succeeds: %+v", jobs)
	}
	before := fake.removeCalls
	if err := rt.RetryDatabaseTeardowns(ctx); err != nil || fake.removeCalls != before {
		t.Fatalf("a finished job must not run again: err=%v calls=%d->%d", err, before, fake.removeCalls)
	}
}

func TestFindProvisionedNodeIgnoresOlderSameNameNode(t *testing.T) {
	t.Parallel()
	started := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	nodes := []store.Node{
		{ID: "old", Name: "edge", CreatedAt: started.Add(-time.Hour)},
		{ID: "new", Name: "edge", CreatedAt: started.Add(time.Minute)},
	}
	n, ok := findProvisionedNode(nodes, "edge", started)
	if !ok || n.ID != "new" {
		t.Fatalf("want the node enrolled after the provision started, got %+v ok=%v", n, ok)
	}
	if _, ok := findProvisionedNode(nodes[:1], "edge", started); ok {
		t.Fatal("a node that predates the provision must not be adopted")
	}
}

func TestGenerateControlPlaneDRKeypair(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/system/control-plane-dr/keys", ``))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"recipient":"age1`) || !strings.Contains(body, "AGE-SECRET-KEY-") {
		t.Fatalf("response must hold an age recipient and identity: %s", body)
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("the private key response must not be cacheable")
	}
}

func TestRestorePlanNotConfigured(t *testing.T) {
	t.Parallel()
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/system/control-plane-dr/restore-plan", ``))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501 without disaster recovery configured", rec.Code)
	}
}
