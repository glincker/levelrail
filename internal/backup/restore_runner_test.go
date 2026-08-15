package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeRestoreHistoryStore struct {
	backups  map[string]store.BackupHistory
	targets  map[string]store.BackupTarget
	started  []store.RestoreHistory
	finished []struct {
		id, status, errMsg, finishedAt string
	}
	getBackupErr error
	getTargetErr error
	startErr     error
	finishErr    error
}

func (f *fakeRestoreHistoryStore) GetBackupHistory(_ context.Context, id string) (store.BackupHistory, error) {
	if f.getBackupErr != nil {
		return store.BackupHistory{}, f.getBackupErr
	}
	b, ok := f.backups[id]
	if !ok {
		return store.BackupHistory{}, store.ErrBackupHistoryNotFound
	}
	return b, nil
}

func (f *fakeRestoreHistoryStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	if f.getTargetErr != nil {
		return store.BackupTarget{}, f.getTargetErr
	}
	t, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return t, nil
}

func (f *fakeRestoreHistoryStore) StartRestoreHistory(_ context.Context, h store.RestoreHistory) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.started = append(f.started, h)
	return nil
}

func (f *fakeRestoreHistoryStore) FinishRestoreHistory(_ context.Context, id, status, errMsg, finishedAt string) error {
	if f.finishErr != nil {
		return f.finishErr
	}
	f.finished = append(f.finished, struct {
		id, status, errMsg, finishedAt string
	}{id, status, errMsg, finishedAt})
	return nil
}

type fakeDownloader struct {
	gotDest Destination
	gotKey  string
	content string
	err     error
}

func (f *fakeDownloader) Download(_ context.Context, dest Destination, key string) (io.ReadCloser, error) {
	f.gotDest = dest
	f.gotKey = key
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), nil
}

type fakeRestorer struct {
	gotEngine, gotContainer string
	gotBody                 string
	err                     error
}

func (f *fakeRestorer) Restore(_ context.Context, engine, containerName string, dump io.Reader) error {
	f.gotEngine = engine
	f.gotContainer = containerName
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, dump)
	f.gotBody = buf.String()
	return f.err
}

func newTestBackup() store.BackupHistory {
	return store.BackupHistory{
		ID:           "bkh_1",
		DatabaseName: "mydb",
		TargetID:     "bkt_test",
		ObjectKey:    "mydb/mydb-20260814T030000Z.dump",
		Status:       store.BackupStatusSucceeded,
	}
}

func TestRestoreRunner_RunRestore_Success(t *testing.T) {
	hs := &fakeRestoreHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	down := &fakeDownloader{content: "dump-bytes"}
	restorer := &fakeRestorer{}
	fixed := time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)
	rr := &RestoreRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: down,
		Restorer:   restorer,
		Now:        func() time.Time { return fixed },
	}

	err := rr.RunRestore(context.Background(), "rsh_1", "mydb", "bkh_1", "postgres", "db-mydb")
	if err != nil {
		t.Fatalf("RunRestore() error = %v", err)
	}

	if len(hs.started) != 1 || hs.started[0].ID != "rsh_1" || hs.started[0].DatabaseName != "mydb" || hs.started[0].BackupHistoryID != "bkh_1" {
		t.Fatalf("StartRestoreHistory call = %+v, want one call for rsh_1/mydb/bkh_1", hs.started)
	}
	if len(hs.finished) != 1 {
		t.Fatalf("FinishRestoreHistory calls = %d, want 1", len(hs.finished))
	}
	got := hs.finished[0]
	if got.status != store.BackupStatusSucceeded {
		t.Errorf("finish status = %q, want %q", got.status, store.BackupStatusSucceeded)
	}
	if got.errMsg != "" {
		t.Errorf("finish errMsg = %q, want empty", got.errMsg)
	}

	if down.gotKey != "mydb/mydb-20260814T030000Z.dump" {
		t.Errorf("download key = %q, want the backup's own object key", down.gotKey)
	}
	if down.gotDest.AccessKeyID != "AKIATEST" || down.gotDest.SecretAccessKey != "shh" {
		t.Errorf("download dest credentials = %+v, want resolved secrets", down.gotDest)
	}
	if restorer.gotEngine != "postgres" || restorer.gotContainer != "db-mydb" {
		t.Errorf("restore called with engine=%q container=%q, want postgres/db-mydb", restorer.gotEngine, restorer.gotContainer)
	}
	if restorer.gotBody != "dump-bytes" {
		t.Errorf("restored body = %q, want %q", restorer.gotBody, "dump-bytes")
	}
}

func TestRestoreRunner_RunRestore_BackupNotFound_RecordsFailure(t *testing.T) {
	hs := &fakeRestoreHistoryStore{backups: map[string]store.BackupHistory{}}
	rr := &RestoreRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{},
		Restorer:   &fakeRestorer{},
	}

	err := rr.RunRestore(context.Background(), "rsh_1", "mydb", "bkh_missing", "postgres", "db-mydb")
	if err == nil {
		t.Fatal("RunRestore() error = nil, want an error for a missing backup")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
}

// TestRestoreRunner_RunRestore_BackupNotSucceeded_RefusesAndRecordsFailure
// is the core safety property RunRestore's own doc comment describes:
// restoring from a backup that never actually succeeded (still running,
// or already failed) is refused outright, never attempted, regardless of
// what internal/api's own synchronous check already did before calling
// this.
func TestRestoreRunner_RunRestore_BackupNotSucceeded_RefusesAndRecordsFailure(t *testing.T) {
	for _, status := range []string{store.BackupStatusRunning, store.BackupStatusFailed} {
		t.Run(status, func(t *testing.T) {
			backup := newTestBackup()
			backup.Status = status
			hs := &fakeRestoreHistoryStore{
				backups: map[string]store.BackupHistory{"bkh_1": backup},
				targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
			}
			down := &fakeDownloader{content: "dump-bytes"}
			restorer := &fakeRestorer{}
			rr := &RestoreRunner{
				Store:      hs,
				Secrets:    newTestSecrets(),
				Downloader: down,
				Restorer:   restorer,
			}

			err := rr.RunRestore(context.Background(), "rsh_1", "mydb", "bkh_1", "postgres", "db-mydb")
			if err == nil {
				t.Fatalf("RunRestore() error = nil, want a refusal for a backup with status %q", status)
			}
			if down.gotKey != "" {
				t.Errorf("Downloader was called (key = %q) for a backup with status %q, want it never invoked", down.gotKey, status)
			}
			if restorer.gotBody != "" {
				t.Errorf("Restorer was called for a backup with status %q, want it never invoked", status)
			}
			if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
				t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
			}
			if hs.finished[0].errMsg == "" {
				t.Error("finish errMsg is empty, want the not-succeeded reason recorded")
			}
		})
	}
}

func TestRestoreRunner_RunRestore_DownloadFails_RecordsFailure_NeverRestores(t *testing.T) {
	hs := &fakeRestoreHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	restorer := &fakeRestorer{}
	rr := &RestoreRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{err: errors.New("bucket: object not found")},
		Restorer:   restorer,
	}

	err := rr.RunRestore(context.Background(), "rsh_1", "mydb", "bkh_1", "postgres", "db-mydb")
	if err == nil {
		t.Fatal("RunRestore() error = nil, want the download failure surfaced")
	}
	if restorer.gotBody != "" {
		t.Error("Restorer was called after a download failure, want it never invoked")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
}

func TestRestoreRunner_RunRestore_RestoreFails_RecordsFailure(t *testing.T) {
	hs := &fakeRestoreHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	rr := &RestoreRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{content: "dump-bytes"},
		Restorer:   &fakeRestorer{err: errors.New("psql: syntax error")},
	}

	err := rr.RunRestore(context.Background(), "rsh_1", "mydb", "bkh_1", "postgres", "db-mydb")
	if err == nil {
		t.Fatal("RunRestore() error = nil, want the restore failure surfaced")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
}

func TestRestoreRunner_RunRestore_SecretResolveFails_NeverDownloads(t *testing.T) {
	hs := &fakeRestoreHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	down := &fakeDownloader{content: "dump-bytes"}
	rr := &RestoreRunner{
		Store:      hs,
		Secrets:    &fakeSecrets{err: errors.New("secrets: master key not configured")},
		Downloader: down,
		Restorer:   &fakeRestorer{},
	}

	err := rr.RunRestore(context.Background(), "rsh_1", "mydb", "bkh_1", "postgres", "db-mydb")
	if err == nil {
		t.Fatal("RunRestore() error = nil, want the secret resolution failure surfaced")
	}
	if down.gotKey != "" {
		t.Error("Downloader was called despite a secret resolution failure, want it skipped")
	}
}
