package archive

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func newFixture(t *testing.T) (*Manager, *store.DB) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "live.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db.DB, dir), db
}

func mustExec(t *testing.T, db *store.DB, q string, args ...any) {
	t.Helper()
	if _, err := db.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("exec %q: %v", q, err)
	}
}

func count(t *testing.T, db *store.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

func seedAudit(t *testing.T, db *store.DB, old, recent int) {
	t.Helper()
	oldAt := store.FormatAuditTime(time.Now().AddDate(0, 0, -90))
	newAt := store.FormatAuditTime(time.Now())
	for i := range old {
		mustExec(t, db, `INSERT INTO audit_log (id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at) VALUES (?, 'user', 'u', 'root', 'write', 'POST', '/x', 200, '1.1.1.1', ?)`, "old-"+string(rune('a'+i)), oldAt)
	}
	for i := range recent {
		mustExec(t, db, `INSERT INTO audit_log (id, actor_type, actor_id, actor_name, ability, method, path, status_code, remote_addr, created_at) VALUES (?, 'user', 'u', 'root', 'write', 'POST', '/x', 200, '1.1.1.1', ?)`, "new-"+string(rune('a'+i)), newAt)
	}
}

func TestArchiveAuditRestoreRoundTrip(t *testing.T) {
	m, db := newFixture(t)
	ctx := context.Background()
	seedAudit(t, db, 5, 2)

	b, err := m.Create(ctx, KindAudit, time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if b.TotalRows != 5 || b.Rows["audit_log"] != 5 {
		t.Fatalf("bundle rows = %+v", b.Rows)
	}
	if got := count(t, db, "audit_log"); got != 2 {
		t.Fatalf("live rows after archive = %d, want 2", got)
	}
	if _, err := m.Create(ctx, KindAudit, time.Now().AddDate(0, 0, -30)); !errors.Is(err, ErrNothingToArchive) {
		t.Fatalf("second create err = %v, want ErrNothingToArchive", err)
	}

	page, err := m.Browse(b.ID, "audit_log", 1, 2)
	if err != nil || len(page.Rows) != 2 || page.Total != 5 {
		t.Fatalf("browse = %+v, err %v", page, err)
	}

	res, err := m.Restore(ctx, b.ID)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if res.Restored["audit_log"] != 5 {
		t.Fatalf("restored = %+v", res)
	}
	if got := count(t, db, "audit_log"); got != 7 {
		t.Fatalf("live rows after restore = %d, want 7", got)
	}
	again, err := m.Restore(ctx, b.ID)
	if err != nil || again.Restored["audit_log"] != 0 || again.Existing != 5 {
		t.Fatalf("idempotent restore = %+v, err %v", again, err)
	}
}

func TestVerifyByRestoreAndTamper(t *testing.T) {
	m, db := newFixture(t)
	ctx := context.Background()
	seedAudit(t, db, 3, 0)
	b, err := m.Create(ctx, KindAudit, time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	v, err := m.Verify(ctx, b.ID)
	if err != nil || v.VerifiedOK == nil || !*v.VerifiedOK {
		t.Fatalf("verify = %+v, err %v", v, err)
	}
	if err := os.WriteFile(m.bundlePath(b.ID), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, err = m.Verify(ctx, b.ID)
	if !errors.Is(err, ErrChecksum) || v.VerifiedOK == nil || *v.VerifiedOK {
		t.Fatalf("tampered verify = %+v, err %v", v, err)
	}
	if _, err := m.Restore(ctx, b.ID); !errors.Is(err, ErrChecksum) {
		t.Fatalf("tampered restore err = %v", err)
	}
}

func TestArchiveKeepsRollbackTargets(t *testing.T) {
	m, db := newFixture(t)
	ctx := context.Background()
	old := time.Now().AddDate(0, 0, -90)
	for i, st := range []string{"succeeded", "succeeded", "succeeded", "failed"} {
		at := old.Add(time.Duration(i) * time.Hour).Format(time.RFC3339Nano)
		mustExec(t, db, `INSERT INTO deploy_attempts (id, service_name, image, status, started_at, finished_at) VALUES (?, 'web', 'img', ?, ?, ?)`, "d"+string(rune('a'+i)), st, at, at)
	}
	b, err := m.Create(ctx, KindDeploys, time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if b.Rows["deploy_attempts"] != 2 {
		t.Fatalf("archived = %+v, want the oldest success and the failure", b.Rows)
	}
	if got := count(t, db, "deploy_attempts"); got != 2 {
		t.Fatalf("live = %d, want the 2 newest successes kept", got)
	}
	if _, err := m.Restore(ctx, b.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if got := count(t, db, "deploy_attempts"); got != 4 {
		t.Fatalf("live after restore = %d", got)
	}
}

func TestArchivePipelinesCascadeAndRestore(t *testing.T) {
	m, db := newFixture(t)
	ctx := context.Background()
	at := time.Now().AddDate(0, 0, -90).Format(time.RFC3339Nano)
	mustExec(t, db, `INSERT INTO pipelines (id, app_name, name, yaml, created_at, updated_at) VALUES ('p1', 'web', 'ci', 'x', ?, ?)`, at, at)
	mustExec(t, db, `INSERT INTO pipeline_runs (id, pipeline_id, app_name, number, trigger_kind, definition, status, created_at) VALUES ('r1', 'p1', 'web', 1, 'manual', '{}', 'succeeded', ?)`, at)
	mustExec(t, db, `INSERT INTO pipeline_runs (id, pipeline_id, app_name, number, trigger_kind, definition, status, created_at) VALUES ('r2', 'p1', 'web', 2, 'manual', '{}', 'running', ?)`, at)
	mustExec(t, db, `INSERT INTO pipeline_jobs (id, run_id, job_key, display_name, status) VALUES ('j1', 'r1', 'build', 'build', 'succeeded')`)
	mustExec(t, db, `INSERT INTO pipeline_steps (job_id, idx, name, kind, status) VALUES ('j1', 0, 'go build', 'run', 'succeeded')`)

	b, err := m.Create(ctx, KindPipelines, time.Now().AddDate(0, 0, -30))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if b.Rows["pipeline_runs"] != 1 || b.Rows["pipeline_jobs"] != 1 || b.Rows["pipeline_steps"] != 1 {
		t.Fatalf("rows = %+v", b.Rows)
	}
	if count(t, db, "pipeline_runs") != 1 || count(t, db, "pipeline_jobs") != 0 || count(t, db, "pipeline_steps") != 0 {
		t.Fatal("archived run did not leave the live tables")
	}
	if _, err := m.Verify(ctx, b.ID); err != nil {
		t.Fatalf("verify: %v", err)
	}
	if _, err := m.Restore(ctx, b.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if count(t, db, "pipeline_runs") != 2 || count(t, db, "pipeline_jobs") != 1 || count(t, db, "pipeline_steps") != 1 {
		t.Fatal("restore did not bring the run back with its jobs and steps")
	}

	if _, err := m.Create(ctx, KindPipelines, time.Now().AddDate(0, 0, -30)); err != nil {
		t.Fatalf("re-archive: %v", err)
	}
	mustExec(t, db, `DELETE FROM pipelines`)
	res, err := m.Restore(ctx, b.ID)
	if err != nil || res.Skipped != 3 {
		t.Fatalf("restore without parent = %+v, err %v", res, err)
	}
}

func TestRunAppliesPolicyAndPrunes(t *testing.T) {
	m, db := newFixture(t)
	ctx := context.Background()
	seedAudit(t, db, 4, 1)
	res, err := m.Run(ctx, store.ArchiveSettings{AuditHotDays: 30, DeployHotDays: 30, PipelineHotDays: 30, KeepDays: 365})
	if err != nil || len(res.Created) != 1 {
		t.Fatalf("run = %+v, err %v", res, err)
	}
	m.now = func() time.Time { return time.Now().AddDate(0, 0, 400) }
	n, err := m.Prune(365)
	if err != nil || n != 1 {
		t.Fatalf("prune = %d, err %v", n, err)
	}
	list, _ := m.List()
	if len(list) != 0 {
		t.Fatalf("bundles after prune = %d", len(list))
	}
}

func TestInvalidIDRejected(t *testing.T) {
	m, _ := newFixture(t)
	for _, id := range []string{"../etc/passwd", "audit-x", ""} {
		if _, err := m.Get(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Get(%q) err = %v", id, err)
		}
	}
}
