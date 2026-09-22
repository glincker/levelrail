package backup

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// TestPITR_Postgres_Live_RestoresBeforeMarkerNotAfterMarker is the
// strongest proof available that point-in-time restore actually works,
// against a real Postgres container, no mock or fake anywhere in the
// path: provision a PITR-enabled database exactly the way
// internal/reconcile/database's own controller does (wal_level=replica,
// archive_mode=on, archive_command copying into a mounted wal-archive
// volume), seed a "before" marker row, take a real physical base backup
// (ContainerBaseBackuper), record a target timestamp guaranteed already
// archived (RecoverableWindowEnd), seed an "after" marker row that must
// NOT survive, stop the container, wipe and repopulate its data volume
// from the captured base backup targeting the recorded timestamp
// (ContainerPITRRestorer), restart it, wait for Postgres to actually
// finish archive recovery and promote, then query the live database
// directly: the before marker must be present and the after marker must
// be gone.
func TestPITR_Postgres_Live_RestoresBeforeMarkerNotAfterMarker(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	// Docker's own volume-remove API has no exported wrapper on
	// docker.Runtime today (dataVolumeName's own doc comment,
	// internal/reconcile/database/controller.go, explains why: volumes
	// are deliberately never removed in production, only ever reused
	// across replacements). A random per-run suffix, rather than trying
	// to clean up the named volumes this test creates, sidesteps that
	// gap the same way: each run gets its own dataVol/walVol, so a
	// previous run's leftover volume (a real Postgres data directory,
	// never safe to silently reuse for a fresh CREATE TABLE) can never
	// collide with this one.
	suffix, err := randomHelperSuffix()
	if err != nil {
		t.Fatalf("generate test suffix: %v", err)
	}
	name := "levelrail-test-pitr-postgres-" + suffix
	dataVol := name + "-data"
	walVol := name + "-wal-archive"
	removeContainerIfExists(ctx, t, rt, name)
	t.Cleanup(func() { removeContainerIfExists(context.Background(), t, rt, name) })

	createAndStartPITRContainer(ctx, t, rt, name, dataVol, walVol)

	waitReady(ctx, t, rt, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", "SELECT 1"}, 30*time.Second)
	fixWALArchivePermissions(ctx, t, rt, name)

	beforeMarker := "levelrail-pitr-live-before-3f8a"
	runSQL(ctx, t, rt, name, `CREATE TABLE pitr_probe (val text); INSERT INTO pitr_probe VALUES ('`+beforeMarker+`');`)

	baseBackuper := &ContainerBaseBackuper{Runtime: rt}
	tarStream, err := baseBackuper.BaseBackup(ctx, name)
	if err != nil {
		t.Fatalf("BaseBackup() error = %v", err)
	}
	var tarBuf bytes.Buffer
	if _, err := io.Copy(&tarBuf, tarStream); err != nil {
		t.Fatalf("reading base backup stream: %v", err)
	}
	_ = tarStream.Close()
	if tarBuf.Len() < 1000 {
		t.Fatalf("base backup tar suspiciously small: %d bytes", tarBuf.Len())
	}

	target, err := RecoverableWindowEnd(ctx, rt, name)
	if err != nil {
		t.Fatalf("RecoverableWindowEnd() error = %v", err)
	}

	afterMarker := "levelrail-pitr-live-after-9c21"
	runSQL(ctx, t, rt, name, `INSERT INTO pitr_probe VALUES ('`+afterMarker+`');`)

	// Force the "after" write archived too, so this test proves a real
	// point-in-time choice (restore to target, not merely "restore to
	// whatever the last archived WAL happens to be").
	if _, err := RecoverableWindowEnd(ctx, rt, name); err != nil {
		t.Fatalf("RecoverableWindowEnd() (post-after-marker) error = %v", err)
	}

	state, err := rt.InspectByName(ctx, name)
	if err != nil || state == nil {
		t.Fatalf("InspectByName() before stop = (%v, %v)", state, err)
	}
	if err := rt.Stop(ctx, state.ID, 30*time.Second); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	restorer := &ContainerPITRRestorer{Runtime: rt}
	if err := restorer.Restore(ctx, dataVol, bytes.NewReader(tarBuf.Bytes()), target); err != nil {
		t.Fatalf("Restore() error = %v", err)
	}

	if err := rt.Start(ctx, state.ID); err != nil {
		t.Fatalf("Start() (after restore) error = %v", err)
	}

	waitPromotedLive(ctx, t, rt, name, 60*time.Second)

	out := runSQLCapture(ctx, t, rt, name, `SELECT val FROM pitr_probe ORDER BY val;`)
	if !strings.Contains(out, beforeMarker) {
		t.Errorf("restored database is missing the before marker %q; got rows: %q", beforeMarker, out)
	}
	if strings.Contains(out, afterMarker) {
		t.Errorf("restored database still contains the after marker %q, which must NOT have survived a restore targeting a point before it was written; got rows: %q", afterMarker, out)
	}

	count := strings.TrimSpace(runSQLCapture(ctx, t, rt, name, `SELECT count(*) FROM pitr_probe;`))
	if count != "1" {
		t.Errorf("row count after restore = %q, want exactly 1 (only the before marker)", count)
	}
}

func createAndStartPITRContainer(ctx context.Context, t *testing.T, rt docker.Runtime, name, dataVol, walVol string) {
	t.Helper()
	if err := rt.EnsureVolume(ctx, dataVol); err != nil {
		t.Fatalf("EnsureVolume(data) error = %v", err)
	}
	if err := rt.EnsureVolume(ctx, walVol); err != nil {
		t.Fatalf("EnsureVolume(wal) error = %v", err)
	}

	id, err := rt.Create(ctx, docker.ContainerSpec{
		Name:  name,
		Image: "postgres:16",
		Env: map[string]string{
			"POSTGRES_USER":     "leveltest",
			"POSTGRES_PASSWORD": "leveltestpass",
		},
		Volumes: []docker.VolumeMount{
			{Name: dataVol, ContainerPath: "/var/lib/postgresql/data"},
			{Name: walVol, ContainerPath: postgresWALArchivePath},
		},
		// Mirrors internal/reconcile/database's own postgresCommand(nil,
		// true) output exactly (that package's own doc comment there
		// explains archive_timeout=60).
		Command: []string{
			"postgres",
			"-c", "wal_level=replica",
			"-c", "archive_mode=on",
			"-c", "archive_timeout=60",
			"-c", "archive_command=test ! -f " + postgresWALArchivePath + "/%f && cp %p " + postgresWALArchivePath + "/%f",
		},
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := rt.Start(ctx, id); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
}

// fixWALArchivePermissions mirrors internal/reconcile/database's own
// ensureWALArchiveWritable (controller.go): a fresh named Docker volume
// mounts root:root, which archive_command cannot write into as the
// postgres user until chowned.
func fixWALArchivePermissions(ctx context.Context, t *testing.T, rt docker.Runtime, name string) {
	t.Helper()
	rc, err := rt.Exec(ctx, name, []string{"chown", "-R", "postgres:postgres", postgresWALArchivePath})
	if err != nil {
		t.Fatalf("chown wal archive: %v", err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		t.Fatalf("chown wal archive: %v", err)
	}
}

func runSQL(ctx context.Context, t *testing.T, rt docker.Runtime, name, sql string) {
	t.Helper()
	rc, err := rt.Exec(ctx, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", sql})
	if err != nil {
		t.Fatalf("Exec(%q) error = %v", sql, err)
	}
	defer func() { _ = rc.Close() }()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		t.Fatalf("Exec(%q) drain error = %v", sql, err)
	}
}

func runSQLCapture(ctx context.Context, t *testing.T, rt docker.Runtime, name, sql string) string {
	t.Helper()
	rc, err := rt.Exec(ctx, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-Atq", "-c", sql})
	if err != nil {
		t.Fatalf("Exec(%q) error = %v", sql, err)
	}
	defer func() { _ = rc.Close() }()
	out, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("Exec(%q) read error = %v", sql, err)
	}
	return string(out)
}

// waitPromotedLive polls pg_is_in_recovery() against a real container
// until it reports false (archive recovery finished and promoted) or
// timeout elapses, the live-test counterpart of PITRRunner.waitPromoted
// (pitr_runner.go).
func waitPromotedLive(ctx context.Context, t *testing.T, rt docker.Runtime, name string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		rc, err := rt.Exec(ctx, name, pgIsInRecoveryCmd)
		if err == nil {
			out, readErr := io.ReadAll(rc)
			_ = rc.Close()
			if readErr == nil {
				last = strings.TrimSpace(string(out))
				if last == "f" {
					return
				}
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	t.Fatalf("database in container %q never promoted within %v (last pg_is_in_recovery = %q)", name, timeout, last)
}
