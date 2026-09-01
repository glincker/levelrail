package backup

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeReadyCloneRestoreStore is CloneRestoreStore backed by an
// in-memory dump instead of a real S3-compatible bucket, the same split
// TestContainerRestorer_Restore_Postgres_Live (restore_live_test.go)
// already draws: the object storage leg is not what this test is
// proving, the real Docker mechanics on both sides of it are.
// GetConditions reports Ready immediately, standing in for the real
// reconciler internal/api's own handler would otherwise wait on (out of
// scope here: this test proves CloneRestoreRunner's own restore
// mechanics against two real containers, not the full create-and-
// converge path, which belongs to internal/reconcile/database, a
// package this task does not touch).
type fakeReadyCloneRestoreStore struct {
	backup     store.BackupHistory
	target     store.BackupTarget
	dumpObject []byte
}

func (f *fakeReadyCloneRestoreStore) GetBackupHistory(_ context.Context, id string) (store.BackupHistory, error) {
	if id != f.backup.ID {
		return store.BackupHistory{}, store.ErrBackupHistoryNotFound
	}
	return f.backup, nil
}

func (f *fakeReadyCloneRestoreStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	if id != f.target.ID {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return f.target, nil
}

func (f *fakeReadyCloneRestoreStore) StartCloneRestore(_ context.Context, _ store.CloneRestore) error {
	return nil
}

func (f *fakeReadyCloneRestoreStore) FinishCloneRestore(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (f *fakeReadyCloneRestoreStore) GetConditions(_ context.Context, _ string) ([]reconcile.Condition, error) {
	return []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"}}, nil
}

// inMemoryDownloader hands back a fixed byte slice regardless of
// Destination, the download-leg counterpart to fakeReadyCloneRestoreStore
// above.
type inMemoryDownloader struct {
	content []byte
}

func (d *inMemoryDownloader) Download(_ context.Context, _ Destination, _ string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(d.content)), nil
}

// TestCloneRestoreRunner_RunCloneRestore_Postgres_Live is the strongest
// proof available that "restore as new database" actually restores into
// a different, brand-new container while leaving the source alone, run
// against two real Postgres containers: seed the source, dump it (the
// real backup snapshot), write more data into the source afterward (real
// production traffic that happened after the backup), then restore the
// dump into a second, independently created container via
// CloneRestoreRunner.RunCloneRestore. Two things are then checked
// directly against the live containers, not against in-memory state: the
// new database has exactly the backup-time data (not the post-backup
// write, since that happened after the snapshot), and the source
// database still has both, completely unaffected by the whole operation.
func TestCloneRestoreRunner_RunCloneRestore_Postgres_Live(t *testing.T) {
	rt := liveRuntime(t)
	ctx := context.Background()

	const sourceName = "levelrail-test-clone-restore-source"
	const newName = "levelrail-test-clone-restore-new"
	for _, name := range []string{sourceName, newName} {
		removeContainerIfExists(ctx, t, rt, name)
	}
	t.Cleanup(func() {
		for _, name := range []string{sourceName, newName} {
			removeContainerIfExists(context.Background(), t, rt, name)
		}
	})

	startPostgres := func(name string) {
		t.Helper()
		id, err := rt.Create(ctx, docker.ContainerSpec{
			Name:  name,
			Image: "postgres:16",
			Env: map[string]string{
				"POSTGRES_USER":     "leveltest",
				"POSTGRES_PASSWORD": "leveltestpass",
			},
		})
		if err != nil {
			t.Fatalf("Create(%s) error = %v", name, err)
		}
		if err := rt.Start(ctx, id); err != nil {
			t.Fatalf("Start(%s) error = %v", name, err)
		}
		waitReady(ctx, t, rt, name, []string{"psql", "-U", "leveltest", "-d", "leveltest", "-c", "SELECT 1"}, 30*time.Second)
	}

	startPostgres(sourceName)
	startPostgres(newName)

	runSQL := func(containerName, sql string) {
		t.Helper()
		exec, err := rt.Exec(ctx, containerName, []string{"sh", "-c", `psql -U leveltest -d leveltest -c "` + sql + `"`})
		if err != nil {
			t.Fatalf("Exec(%s) error = %v", containerName, err)
		}
		if _, err := io.Copy(io.Discard, exec); err != nil {
			t.Fatalf("running SQL on %s: %v", containerName, err)
		}
		_ = exec.Close()
	}

	queryValues := func(containerName string) string {
		t.Helper()
		exec, err := rt.Exec(ctx, containerName, []string{"sh", "-c", `psql -U leveltest -d leveltest -t -c "SELECT val FROM clone_probe;"`})
		if err != nil {
			t.Fatalf("Exec(%s) query error = %v", containerName, err)
		}
		out, err := io.ReadAll(exec)
		_ = exec.Close()
		if err != nil {
			t.Fatalf("reading query output from %s: %v", containerName, err)
		}
		return string(out)
	}

	backupTimeMarker := "levelrail-clone-restore-live-backup-time-4d2a"
	runSQL(sourceName, `CREATE TABLE clone_probe (val text); INSERT INTO clone_probe VALUES ('`+backupTimeMarker+`');`)

	d := &ContainerDumper{Runtime: rt}
	dumpStream, err := d.Dump(ctx, store.EnginePostgres, sourceName)
	if err != nil {
		t.Fatalf("Dump() error = %v", err)
	}
	var dumpBuf bytes.Buffer
	if _, err := io.Copy(&dumpBuf, dumpStream); err != nil {
		t.Fatalf("reading Dump() stream error = %v", err)
	}
	_ = dumpStream.Close()
	if !strings.Contains(dumpBuf.String(), backupTimeMarker) {
		t.Fatalf("captured dump does not contain the backup-time marker %q, test setup is broken", backupTimeMarker)
	}

	// Real activity against the source after the backup was taken: the
	// clone-restore attempt below must never see or affect this.
	postBackupMarker := "levelrail-clone-restore-live-post-backup-9f7e"
	runSQL(sourceName, `INSERT INTO clone_probe VALUES ('`+postBackupMarker+`');`)

	cr := &CloneRestoreRunner{
		Store: &fakeReadyCloneRestoreStore{
			backup: store.BackupHistory{
				ID: "bkh_live_1", DatabaseName: sourceName, TargetID: "bkt_test",
				ObjectKey: "source/1.dump", Status: store.BackupStatusSucceeded,
			},
			target: newTestTarget(),
		},
		Secrets:      newTestSecrets(),
		Downloader:   &inMemoryDownloader{content: dumpBuf.Bytes()},
		Restorer:     &ContainerRestorer{Runtime: rt},
		PollInterval: 10 * time.Millisecond,
	}

	err = cr.RunCloneRestore(ctx, "clr_live_1", sourceName, newName, "bkh_live_1", store.EnginePostgres, newName, "database/"+newName)
	if err != nil {
		t.Fatalf("RunCloneRestore() error = %v", err)
	}

	newValues := queryValues(newName)
	if !strings.Contains(newValues, backupTimeMarker) {
		t.Errorf("new database values = %q, want the backup-time marker %q", newValues, backupTimeMarker)
	}
	if strings.Contains(newValues, postBackupMarker) {
		t.Errorf("new database values = %q, want it NOT to contain the post-backup marker %q (that write happened after the snapshot restored here)", newValues, postBackupMarker)
	}

	sourceValues := queryValues(sourceName)
	if !strings.Contains(sourceValues, backupTimeMarker) || !strings.Contains(sourceValues, postBackupMarker) {
		t.Errorf("source database values = %q, want both markers still present: the clone-restore must never touch the source", sourceValues)
	}
}
