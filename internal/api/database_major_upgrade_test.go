package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeMajorUpgrader struct {
	mu       sync.Mutex
	runs     chan [2]string
	rollback chan string
	discard  chan string
}

func newFakeMajorUpgrader() *fakeMajorUpgrader {
	return &fakeMajorUpgrader{runs: make(chan [2]string, 2), rollback: make(chan string, 2), discard: make(chan string, 2)}
}

func (f *fakeMajorUpgrader) RunMajorUpgrade(_ context.Context, _, name, to string) error {
	f.runs <- [2]string{name, to}
	return nil
}

func (f *fakeMajorUpgrader) Rollback(_ context.Context, id string) error {
	f.rollback <- id
	return nil
}

func (f *fakeMajorUpgrader) DiscardSnapshot(_ context.Context, id string) error {
	f.discard <- id
	return nil
}

func TestCheckMajorUpgrade(t *testing.T) {
	pg := func(v string, pitr bool) *store.DesiredDatabase {
		return &store.DesiredDatabase{Name: "main", Engine: store.EnginePostgres, Version: v, PITREnabled: pitr}
	}
	tests := []struct {
		name    string
		db      *store.DesiredDatabase
		to      string
		wantMsg string
	}{
		{"valid upgrade", pg("16", false), "17", ""},
		{"skip a major", pg("15", false), "17", ""},
		{"same major", pg("16", false), "16.4", "unchanged"},
		{"downgrade", pg("17", false), "16", "downgrading"},
		{"pitr enabled", pg("16", true), "17", "point-in-time"},
		{"non numeric target", pg("16", false), "latest", "cannot compare"},
		{"bad tag", pg("16", false), "../x", "image tag"},
		{"plain to pgvector same major", pg("17", false), "17-pgvector", ""},
		{"plain to pgvector newer major", pg("16", false), "17-pgvector", ""},
		{"pgvector to newer pgvector", pg("16-pgvector", false), "17-pgvector", ""},
		{"pgvector back to plain", pg("16-pgvector", false), "17", "pgvector image back"},
		{"malformed variant", pg("16", false), "17.4-pgvector", "not a valid pgvector"},
		{"redis refused", &store.DesiredDatabase{Engine: store.EngineRedis, Version: "7"}, "8", "only available for postgres"},
	}
	for _, tt := range tests {
		got := checkMajorUpgrade(tt.db, tt.to)
		if (tt.wantMsg == "") != (got == "") || !strings.Contains(got, tt.wantMsg) {
			t.Errorf("%s: checkMajorUpgrade() = %q, want to contain %q", tt.name, got, tt.wantMsg)
		}
	}
}

func TestMajorUpgradeRoutes(t *testing.T) {
	up := newFakeMajorUpgrader()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithMajorUpgrader(up))
	cookie := loginTestSession(t, rt, db)
	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"postgres","version":"16"}`)

	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
		return rec
	}

	if rec := do(http.MethodPost, "/api/v1/databases/main/major-upgrade", `{"version":"17"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("missing confirm: status = %d, want 400", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/databases/main/major-upgrade", `{"version":"16.4","confirm":"main"}`); rec.Code != http.StatusConflict {
		t.Errorf("same major: status = %d, want 409", rec.Code)
	}
	rec := do(http.MethodPost, "/api/v1/databases/main/major-upgrade", `{"version":"17","confirm":"main"}`)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("upgrade: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	select {
	case got := <-up.runs:
		if got != [2]string{"main", "17"} {
			t.Errorf("run = %v", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("upgrade never started")
	}

	if err := db.StartMajorUpgrade(context.Background(), store.MajorUpgrade{ID: "mu_done", DatabaseName: "main", FromVersion: "16", ToVersion: "17", StartedAt: "2026-10-05T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if rec := do(http.MethodPost, "/api/v1/databases/main/major-upgrade", `{"version":"18","confirm":"main"}`); rec.Code != http.StatusConflict {
		t.Errorf("concurrent upgrade: status = %d, want 409", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/databases/main/major-upgrades/mu_done/rollback", `{"confirm":"main"}`); rec.Code != http.StatusConflict {
		t.Errorf("rollback while running: status = %d, want 409", rec.Code)
	}

	_ = db.UpdateMajorUpgradePhase(context.Background(), "mu_done", "verify", "db-main-data-pre16-x")
	_ = db.FinishMajorUpgrade(context.Background(), "mu_done", store.BackupStatusSucceeded, "", "2026-10-05T00:05:00Z")

	if rec := do(http.MethodPost, "/api/v1/databases/main/major-upgrades/mu_done/rollback", `{"confirm":"nope"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("rollback bad confirm: status = %d, want 400", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/databases/main/major-upgrades/missing/rollback", `{"confirm":"main"}`); rec.Code != http.StatusNotFound {
		t.Errorf("rollback unknown id: status = %d, want 404", rec.Code)
	}
	if rec := do(http.MethodPost, "/api/v1/databases/main/major-upgrades/mu_done/rollback", `{"confirm":"main"}`); rec.Code != http.StatusAccepted {
		t.Fatalf("rollback: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := <-up.rollback; got != "mu_done" {
		t.Errorf("rollback id = %q", got)
	}
	if rec := do(http.MethodDelete, "/api/v1/databases/main/major-upgrades/mu_done/snapshot", ""); rec.Code != http.StatusNoContent {
		t.Errorf("discard: status = %d, want 204", rec.Code)
	}
	if got := <-up.discard; got != "mu_done" {
		t.Errorf("discard id = %q", got)
	}

	list := do(http.MethodGet, "/api/v1/databases/main/major-upgrades", "")
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"id":"mu_done"`) || !strings.Contains(list.Body.String(), `"snapshot_volume":"db-main-data-pre16-x"`) {
		t.Errorf("list: status = %d, body = %s", list.Code, list.Body.String())
	}
}

func TestMajorUpgradeRoutes_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	mustCreateDatabase(t, rt, cookie, `{"name":"main","engine":"postgres","version":"16"}`)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/databases/main/major-upgrade", `{"version":"17","confirm":"main"}`))
	if rec.Code != http.StatusNotImplemented {
		t.Errorf("status = %d, want 501", rec.Code)
	}
}
