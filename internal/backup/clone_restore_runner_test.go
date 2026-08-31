package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeCloneRestoreStore struct {
	backups  map[string]store.BackupHistory
	targets  map[string]store.BackupTarget
	started  []store.CloneRestore
	finished []struct {
		id, status, errMsg, finishedAt string
	}
	getBackupErr error
	getTargetErr error
	startErr     error
	finishErr    error

	// conditions is consulted once per waitUntilReady poll; conditionsErr
	// short-circuits every call, readyAfterCalls delays a Ready
	// condition until that many calls have happened (0 means ready on
	// the first call), and calls counts how many GetConditions calls
	// this fake actually received.
	conditions      []reconcile.Condition
	conditionsErr   error
	readyAfterCalls int
	calls           int
}

func (f *fakeCloneRestoreStore) GetBackupHistory(_ context.Context, id string) (store.BackupHistory, error) {
	if f.getBackupErr != nil {
		return store.BackupHistory{}, f.getBackupErr
	}
	b, ok := f.backups[id]
	if !ok {
		return store.BackupHistory{}, store.ErrBackupHistoryNotFound
	}
	return b, nil
}

func (f *fakeCloneRestoreStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	if f.getTargetErr != nil {
		return store.BackupTarget{}, f.getTargetErr
	}
	t, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return t, nil
}

func (f *fakeCloneRestoreStore) StartCloneRestore(_ context.Context, h store.CloneRestore) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.started = append(f.started, h)
	return nil
}

func (f *fakeCloneRestoreStore) FinishCloneRestore(_ context.Context, id, status, errMsg, finishedAt string) error {
	if f.finishErr != nil {
		return f.finishErr
	}
	f.finished = append(f.finished, struct {
		id, status, errMsg, finishedAt string
	}{id, status, errMsg, finishedAt})
	return nil
}

func (f *fakeCloneRestoreStore) GetConditions(_ context.Context, _ string) ([]reconcile.Condition, error) {
	f.calls++
	if f.conditionsErr != nil {
		return nil, f.conditionsErr
	}
	if f.calls <= f.readyAfterCalls {
		return []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "Starting"}}, nil
	}
	return f.conditions, nil
}

func newReadyCloneRestoreStore() *fakeCloneRestoreStore {
	return &fakeCloneRestoreStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
		conditions: []reconcile.Condition{
			{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "Running"},
		},
	}
}

func TestCloneRestoreRunner_RunCloneRestore_Success(t *testing.T) {
	hs := newReadyCloneRestoreStore()
	down := &fakeDownloader{content: "dump-bytes"}
	restorer := &fakeRestorer{}
	fixed := time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)
	cr := &CloneRestoreRunner{
		Store:        hs,
		Secrets:      newTestSecrets(),
		Downloader:   down,
		Restorer:     restorer,
		Now:          func() time.Time { return fixed },
		PollInterval: time.Millisecond,
	}

	err := cr.RunCloneRestore(context.Background(), "clr_1", "mydb", "mydb-staging", "bkh_1", "postgres", "db-mydb-staging", "database/mydb-staging")
	if err != nil {
		t.Fatalf("RunCloneRestore() error = %v", err)
	}

	if len(hs.started) != 1 || hs.started[0].ID != "clr_1" || hs.started[0].SourceDatabaseName != "mydb" || hs.started[0].NewDatabaseName != "mydb-staging" {
		t.Fatalf("StartCloneRestore call = %+v, want one call for clr_1/mydb/mydb-staging", hs.started)
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusSucceeded {
		t.Fatalf("FinishCloneRestore calls = %+v, want one succeeded call", hs.finished)
	}
	if restorer.gotEngine != "postgres" || restorer.gotContainer != "db-mydb-staging" {
		t.Errorf("restore called with engine=%q container=%q, want postgres/db-mydb-staging", restorer.gotEngine, restorer.gotContainer)
	}
	if restorer.gotBody != "dump-bytes" {
		t.Errorf("restored body = %q, want %q", restorer.gotBody, "dump-bytes")
	}
}

// TestCloneRestoreRunner_RunCloneRestore_WaitsUntilReady proves the wait
// loop actually polls more than once before proceeding: the new
// database's own conditions start False and only flip True after a few
// calls, so a runner that restored immediately (skipping the wait
// entirely) would pass this test's "restorer never called" check for the
// wrong reason and its own "restored after 3 calls" check would fail.
func TestCloneRestoreRunner_RunCloneRestore_WaitsUntilReady(t *testing.T) {
	hs := newReadyCloneRestoreStore()
	hs.readyAfterCalls = 3
	restorer := &fakeRestorer{}
	cr := &CloneRestoreRunner{
		Store:        hs,
		Secrets:      newTestSecrets(),
		Downloader:   &fakeDownloader{content: "dump-bytes"},
		Restorer:     restorer,
		PollInterval: time.Millisecond,
	}

	err := cr.RunCloneRestore(context.Background(), "clr_1", "mydb", "mydb-staging", "bkh_1", "postgres", "db-mydb-staging", "database/mydb-staging")
	if err != nil {
		t.Fatalf("RunCloneRestore() error = %v", err)
	}
	if hs.calls < 4 {
		t.Errorf("GetConditions calls = %d, want at least 4 (3 not-ready + 1 ready)", hs.calls)
	}
	if restorer.gotBody != "dump-bytes" {
		t.Error("Restorer was never called despite the database eventually becoming ready")
	}
}

// TestCloneRestoreRunner_RunCloneRestore_NeverBecomesReady_RecordsFailure
// is the timeout path: a database whose conditions never report Ready
// (the credentials-not-configured gap internal/reconcile/database's own
// package doc comment describes is a realistic real-world cause) fails
// the clone-restore attempt outright rather than hanging forever, and the
// restorer is never invoked against a database that was never actually
// up.
func TestCloneRestoreRunner_RunCloneRestore_NeverBecomesReady_RecordsFailure(t *testing.T) {
	hs := &fakeCloneRestoreStore{
		backups:    map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets:    map[string]store.BackupTarget{"bkt_test": newTestTarget()},
		conditions: []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionFalse, Reason: "CredentialsNotConfigured"}},
	}
	restorer := &fakeRestorer{}
	cr := &CloneRestoreRunner{
		Store:        hs,
		Secrets:      newTestSecrets(),
		Downloader:   &fakeDownloader{content: "dump-bytes"},
		Restorer:     restorer,
		ReadyTimeout: 20 * time.Millisecond,
		PollInterval: 5 * time.Millisecond,
	}

	err := cr.RunCloneRestore(context.Background(), "clr_1", "mydb", "mydb-staging", "bkh_1", "postgres", "db-mydb-staging", "database/mydb-staging")
	if err == nil {
		t.Fatal("RunCloneRestore() error = nil, want a timeout error")
	}
	if restorer.gotBody != "" {
		t.Error("Restorer was called despite the database never becoming ready")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
	if hs.finished[0].errMsg == "" {
		t.Error("finish errMsg is empty, want the timeout reason recorded")
	}
}

func TestCloneRestoreRunner_RunCloneRestore_BackupNotSucceeded_RefusesAndRecordsFailure(t *testing.T) {
	backup := newTestBackup()
	backup.Status = store.BackupStatusFailed
	hs := newReadyCloneRestoreStore()
	hs.backups["bkh_1"] = backup
	restorer := &fakeRestorer{}
	cr := &CloneRestoreRunner{
		Store:        hs,
		Secrets:      newTestSecrets(),
		Downloader:   &fakeDownloader{content: "dump-bytes"},
		Restorer:     restorer,
		PollInterval: time.Millisecond,
	}

	err := cr.RunCloneRestore(context.Background(), "clr_1", "mydb", "mydb-staging", "bkh_1", "postgres", "db-mydb-staging", "database/mydb-staging")
	if err == nil {
		t.Fatal("RunCloneRestore() error = nil, want a refusal for a backup that never succeeded")
	}
	if restorer.gotBody != "" {
		t.Error("Restorer was called for a backup that never succeeded, want it never invoked")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
}

func TestCloneRestoreRunner_RunCloneRestore_RestoreFails_RecordsFailure(t *testing.T) {
	hs := newReadyCloneRestoreStore()
	cr := &CloneRestoreRunner{
		Store:        hs,
		Secrets:      newTestSecrets(),
		Downloader:   &fakeDownloader{content: "dump-bytes"},
		Restorer:     &fakeRestorer{err: errors.New("psql: syntax error")},
		PollInterval: time.Millisecond,
	}

	err := cr.RunCloneRestore(context.Background(), "clr_1", "mydb", "mydb-staging", "bkh_1", "postgres", "db-mydb-staging", "database/mydb-staging")
	if err == nil {
		t.Fatal("RunCloneRestore() error = nil, want the restore failure surfaced")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
}
