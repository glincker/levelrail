package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeBackupProtection struct {
	health     []backup.ResourceHealth
	exists     bool
	known      bool
	drillErr   error
	drillCalls []string
	restores   chan [3]string
}

func (f *fakeBackupProtection) Health(context.Context) ([]backup.ResourceHealth, error) {
	return f.health, nil
}
func (f *fakeBackupProtection) Encryption() backup.EncryptionStatus {
	return backup.EncryptionStatus{Enabled: true, Recipient: "age1abc", Codec: backup.CodecZstdAge}
}
func (f *fakeBackupProtection) StartDrill(_ context.Context, id string) (string, error) {
	f.drillCalls = append(f.drillCalls, id)
	return "bkd_1", f.drillErr
}
func (f *fakeBackupProtection) RefreshProtection(_ context.Context, ids []string) ([]store.BackupTargetProtection, error) {
	out := make([]store.BackupTargetProtection, 0, len(ids))
	for _, id := range ids {
		out = append(out, store.BackupTargetProtection{TargetID: id, CanDelete: true, CheckedAt: "2026-10-01T00:00:00Z"})
	}
	return out, nil
}
func (f *fakeBackupProtection) VolumeExists(context.Context, docker.Runtime, string) (bool, bool, error) {
	return f.exists, f.known, nil
}
func (f *fakeBackupProtection) RestoreVolumeTo(_ context.Context, _ docker.Runtime, _, svc, vol, newVol, backupID string) error {
	f.restores <- [3]string{svc + "/" + vol, newVol, backupID}
	return nil
}

type bareRuntime struct{ docker.Runtime }

func newProtectionRouter(t *testing.T, p *fakeBackupProtection, nodeErr error) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	resolver := func(string) (docker.Runtime, error) {
		if nodeErr != nil {
			return nil, nodeErr
		}
		return bareRuntime{}, nil
	}
	return NewRouter(logger, testBrand(), db, WithBackupProtection(p, db), WithExecRuntime(resolver)), db
}

func seedVolumeBackup(t *testing.T, db *store.DB, id, status string) {
	t.Helper()
	target := seedBackupTargetForAPI(t, db)
	ctx := context.Background()
	if err := db.StartBackupHistory(ctx, store.BackupHistory{ID: id, ResourceKind: store.BackupResourceKindVolume, ServiceName: "web", VolumeName: "data", TargetID: target.ID, ObjectKey: "k/" + id, StartedAt: "2026-08-14T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if status != store.BackupStatusRunning {
		if err := db.FinishBackupHistory(ctx, id, status, 10, "sum", "", "2026-08-14T00:01:00Z"); err != nil {
			t.Fatal(err)
		}
	}
}

func do(t *testing.T, rt *Router, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func TestBackupProtectionRoutes_RequireAuth(t *testing.T) {
	rt, _ := newTestRouter(t)
	assertRoutesRequireAuth(t, rt, []routeCase{
		{http.MethodGet, "/api/v1/backups/health"},
		{http.MethodGet, "/api/v1/backups/drills"},
		{http.MethodPost, "/api/v1/backups/drills"},
		{http.MethodGet, "/api/v1/backups/drills/bkd_1"},
		{http.MethodPost, "/api/v1/backups/protection/refresh"},
		{http.MethodGet, "/api/v1/apps/web/volumes/data/backup-policy"},
		{http.MethodPut, "/api/v1/apps/web/volumes/data/backup-policy"},
		{http.MethodPost, "/api/v1/apps/web/volumes/data/restore-to"},
	})
}

func TestBackupProtection_NotConfigured(t *testing.T) {
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	for _, c := range []struct{ method, path, body string }{
		{http.MethodGet, "/api/v1/backups/health", ""},
		{http.MethodGet, "/api/v1/backups/drills", ""},
		{http.MethodPost, "/api/v1/backups/drills", `{"backup_id":"x"}`},
		{http.MethodGet, "/api/v1/apps/web/volumes/data/backup-policy", ""},
		{http.MethodPost, "/api/v1/apps/web/volumes/data/restore-to", `{"backup_id":"x"}`},
	} {
		if rec := do(t, rt, cookie, c.method, c.path, c.body); rec.Code != http.StatusNotImplemented {
			t.Fatalf("%s %s = %d, want 501", c.method, c.path, rec.Code)
		}
	}
}

func TestHandleBackupHealth(t *testing.T) {
	p := &fakeBackupProtection{health: []backup.ResourceHealth{{
		Kind: store.BackupResourceKindVolume, AppName: "web", ResourceName: "data", BackupCount: 3, TotalBytes: 99,
		State: backup.HealthHealthy, Encrypted: true,
		LastBackup:          &backup.HealthBackup{ID: "bkh_1", At: "2026-10-01T00:00:00Z", Status: "succeeded", SizeBytes: 33},
		LastVerifiedRestore: &backup.DrillSummary{ID: "bkd_1", At: "2026-10-02T00:00:00Z", Status: "passed", Files: 4},
	}}}
	rt, db := newProtectionRouter(t, p, nil)
	cookie := loginTestSession(t, rt, db)
	if err := db.SetBackupTargetProtection(context.Background(), store.BackupTargetProtection{TargetID: "bkt_1", CanDelete: true, CheckedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	rec := do(t, rt, cookie, http.MethodGet, "/api/v1/backups/health", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var got backupHealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	r := got.Resources[0]
	if r.AppName != "web" || r.LastVerifiedRestore == nil || r.LastVerifiedRestore.Files != 4 || r.LastBackup.SizeBytes != 33 || !r.Encrypted {
		t.Fatalf("resource = %+v", r)
	}
	if !got.Encryption.Enabled || got.Encryption.Recipient != "age1abc" {
		t.Fatalf("encryption = %+v", got.Encryption)
	}
	if len(got.Targets) != 1 || got.Targets[0].Level != backup.ProtectionOpen || !strings.Contains(got.Targets[0].Warning, "can delete or overwrite") {
		t.Fatalf("targets = %+v", got.Targets)
	}
}

func TestHandleStartBackupDrill(t *testing.T) {
	p := &fakeBackupProtection{}
	rt, db := newProtectionRouter(t, p, nil)
	cookie := loginTestSession(t, rt, db)
	seedVolumeBackup(t, db, "ok", store.BackupStatusSucceeded)

	cases := []struct {
		name, body string
		want       int
	}{
		{"missing id", `{}`, http.StatusBadRequest},
		{"unknown backup", `{"backup_id":"nope"}`, http.StatusNotFound},
		{"valid", `{"backup_id":"ok"}`, http.StatusAccepted},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/backups/drills", c.body); rec.Code != c.want {
				t.Fatalf("status = %d, want %d: %s", rec.Code, c.want, rec.Body.String())
			}
		})
	}
	if len(p.drillCalls) != 1 || p.drillCalls[0] != "ok" {
		t.Fatalf("StartDrill calls = %v", p.drillCalls)
	}
}

func TestHandleStartBackupDrill_RefusesFailedBackup(t *testing.T) {
	p := &fakeBackupProtection{}
	rt, db := newProtectionRouter(t, p, nil)
	cookie := loginTestSession(t, rt, db)
	seedVolumeBackup(t, db, "bad", store.BackupStatusFailed)
	if rec := do(t, rt, cookie, http.MethodPost, "/api/v1/backups/drills", `{"backup_id":"bad"}`); rec.Code != http.StatusConflict {
		t.Fatalf("status = %d", rec.Code)
	}
	if len(p.drillCalls) != 0 {
		t.Fatal("a failed backup must not start a drill")
	}
}

func TestHandleListAndGetBackupDrills(t *testing.T) {
	rt, db := newProtectionRouter(t, &fakeBackupProtection{}, nil)
	cookie := loginTestSession(t, rt, db)
	ctx := context.Background()
	if err := db.StartBackupDrill(ctx, store.BackupDrill{ID: "bkd_1", BackupHistoryID: "b", ResourceKind: "volume", ServiceName: "web", VolumeName: "data", StartedAt: "2026-10-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if rec := do(t, rt, cookie, http.MethodGet, "/api/v1/backups/drills?service=web&volume=data", ""); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "bkd_1") {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	if rec := do(t, rt, cookie, http.MethodGet, "/api/v1/backups/drills?limit=9999", ""); rec.Code != http.StatusBadRequest {
		t.Fatalf("limit bound = %d", rec.Code)
	}
	if rec := do(t, rt, cookie, http.MethodGet, "/api/v1/backups/drills/bkd_1", ""); rec.Code != http.StatusOK {
		t.Fatalf("get = %d", rec.Code)
	}
	if rec := do(t, rt, cookie, http.MethodGet, "/api/v1/backups/drills/missing", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("get missing = %d", rec.Code)
	}
}

func TestHandleVolumeBackupPolicy(t *testing.T) {
	rt, db := newProtectionRouter(t, &fakeBackupProtection{}, nil)
	cookie := loginTestSession(t, rt, db)
	seedServiceWithVolume(t, db)
	const path = "/api/v1/apps/web/volumes/data/backup-policy"

	var empty volumeBackupPolicyResource
	rec := do(t, rt, cookie, http.MethodGet, path, "")
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &empty) != nil || empty.RetainDaily != 0 {
		t.Fatalf("empty get = %d %s", rec.Code, rec.Body.String())
	}

	bad := []string{
		`{"retain_daily":-1}`,
		`{"quiesce":"freeze"}`,
		`{"pre_hook":"` + strings.Repeat("x", maxHookLength+1) + `"}`,
	}
	for _, b := range bad {
		if rec := do(t, rt, cookie, http.MethodPut, path, b); rec.Code != http.StatusBadRequest {
			t.Fatalf("PUT %.40s = %d, want 400", b, rec.Code)
		}
	}

	rec = do(t, rt, cookie, http.MethodPut, path, `{"retain_daily":7,"retain_weekly":4,"retain_monthly":6,"pre_hook":" sync ","quiesce":"pause"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body.String())
	}
	stored, err := db.GetVolumeBackupPolicy(context.Background(), "web", "data")
	if err != nil || stored.RetainDaily != 7 || stored.RetainMonthly != 6 || stored.PreHook != "sync" || stored.Quiesce != "pause" {
		t.Fatalf("stored = %+v err = %v", stored, err)
	}
	if rec := do(t, rt, cookie, http.MethodGet, "/api/v1/apps/ghost/volumes/data/backup-policy", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", rec.Code)
	}
}

func TestHandleVolumeRestoreTo(t *testing.T) {
	const path = "/api/v1/apps/web/volumes/data/restore-to"
	await := func(t *testing.T, p *fakeBackupProtection) [3]string {
		t.Helper()
		select {
		case c := <-p.restores:
			return c
		case <-time.After(2 * time.Second):
			t.Fatal("RestoreVolumeTo was not called")
			return [3]string{}
		}
	}
	newEnv := func(t *testing.T, p *fakeBackupProtection, nodeErr error) (*Router, *http.Cookie) {
		rt, db := newProtectionRouter(t, p, nodeErr)
		cookie := loginTestSession(t, rt, db)
		seedServiceWithVolume(t, db)
		seedVolumeBackup(t, db, "bkh_1", store.BackupStatusSucceeded)
		return rt, cookie
	}

	t.Run("generated name never overwrites", func(t *testing.T) {
		p := &fakeBackupProtection{known: true, restores: make(chan [3]string, 1)}
		rt, cookie := newEnv(t, p, nil)
		rec := do(t, rt, cookie, http.MethodPost, path, `{"backup_id":"bkh_1"}`)
		if rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		got := await(t, p)
		if got[0] != "web/data" || !strings.HasPrefix(got[1], "clone-web-data-") || got[2] != "bkh_1" {
			t.Fatalf("call = %v", got)
		}
	})
	t.Run("target app names the volume for that app", func(t *testing.T) {
		p := &fakeBackupProtection{known: true, restores: make(chan [3]string, 1)}
		rt, cookie := newEnv(t, p, nil)
		if rec := do(t, rt, cookie, http.MethodPost, path, `{"backup_id":"bkh_1","target_app":"web-copy"}`); rec.Code != http.StatusAccepted {
			t.Fatalf("status = %d %s", rec.Code, rec.Body.String())
		}
		if got := await(t, p); got[1] != "app-web-copy-data" {
			t.Fatalf("new volume = %q", got[1])
		}
	})
	t.Run("existing volume is refused", func(t *testing.T) {
		p := &fakeBackupProtection{exists: true, known: true, restores: make(chan [3]string, 1)}
		rt, cookie := newEnv(t, p, nil)
		rec := do(t, rt, cookie, http.MethodPost, path, `{"backup_id":"bkh_1","new_volume_name":"in-use"}`)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status = %d", rec.Code)
		}
		select {
		case <-p.restores:
			t.Fatal("restore ran against an existing volume")
		case <-time.After(100 * time.Millisecond):
		}
	})
	t.Run("explicit name on a node that cannot confirm is refused", func(t *testing.T) {
		p := &fakeBackupProtection{known: false, restores: make(chan [3]string, 1)}
		rt, cookie := newEnv(t, p, nil)
		if rec := do(t, rt, cookie, http.MethodPost, path, `{"backup_id":"bkh_1","new_volume_name":"fresh"}`); rec.Code != http.StatusConflict {
			t.Fatalf("status = %d", rec.Code)
		}
	})
	t.Run("unreachable node", func(t *testing.T) {
		p := &fakeBackupProtection{restores: make(chan [3]string, 1)}
		rt, cookie := newEnv(t, p, errors.New("offline"))
		if rec := do(t, rt, cookie, http.MethodPost, path, `{"backup_id":"bkh_1","node_id":"n2"}`); rec.Code != http.StatusBadGateway {
			t.Fatalf("status = %d", rec.Code)
		}
	})
	t.Run("invalid name and foreign backup", func(t *testing.T) {
		p := &fakeBackupProtection{known: true, restores: make(chan [3]string, 1)}
		rt, cookie := newEnv(t, p, nil)
		if rec := do(t, rt, cookie, http.MethodPost, path, `{"backup_id":"bkh_1","new_volume_name":"bad name!"}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid name status = %d", rec.Code)
		}
		if rec := do(t, rt, cookie, http.MethodPost, path, `{"backup_id":"nope"}`); rec.Code != http.StatusNotFound {
			t.Fatalf("unknown backup status = %d", rec.Code)
		}
		if rec := do(t, rt, cookie, http.MethodPost, path, `{}`); rec.Code != http.StatusBadRequest {
			t.Fatalf("missing id status = %d", rec.Code)
		}
	})
}
