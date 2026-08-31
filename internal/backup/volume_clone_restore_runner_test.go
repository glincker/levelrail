package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeVolumeCloneRestoreStore is a hand-written fake for
// VolumeCloneRestoreStore, mirroring fakeCloneRestoreStore's own shape
// minus the reconcile-conditions polling: a volume clone-restore never
// waits on a controller, see VolumeCreator's own doc comment.
type fakeVolumeCloneRestoreStore struct {
	backups  map[string]store.BackupHistory
	targets  map[string]store.BackupTarget
	started  []store.VolumeCloneRestore
	finished []struct {
		id, status, errMsg, finishedAt string
	}
	getBackupErr error
	getTargetErr error
	startErr     error
	finishErr    error
}

func (f *fakeVolumeCloneRestoreStore) GetBackupHistory(_ context.Context, id string) (store.BackupHistory, error) {
	if f.getBackupErr != nil {
		return store.BackupHistory{}, f.getBackupErr
	}
	b, ok := f.backups[id]
	if !ok {
		return store.BackupHistory{}, store.ErrBackupHistoryNotFound
	}
	return b, nil
}

func (f *fakeVolumeCloneRestoreStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	if f.getTargetErr != nil {
		return store.BackupTarget{}, f.getTargetErr
	}
	t, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return t, nil
}

func (f *fakeVolumeCloneRestoreStore) StartVolumeCloneRestore(_ context.Context, h store.VolumeCloneRestore) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.started = append(f.started, h)
	return nil
}

func (f *fakeVolumeCloneRestoreStore) FinishVolumeCloneRestore(_ context.Context, id, status, errMsg, finishedAt string) error {
	if f.finishErr != nil {
		return f.finishErr
	}
	f.finished = append(f.finished, struct {
		id, status, errMsg, finishedAt string
	}{id, status, errMsg, finishedAt})
	return nil
}

func newReadyVolumeCloneRestoreStore() *fakeVolumeCloneRestoreStore {
	return &fakeVolumeCloneRestoreStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestVolumeBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
}

// fakeVolumeCreator is a hand-written fake for VolumeCreator.
type fakeVolumeCreator struct {
	created []string
	err     error
}

func (f *fakeVolumeCreator) EnsureVolume(_ context.Context, name string) error {
	f.created = append(f.created, name)
	return f.err
}

func TestVolumeCloneRestoreRunner_RunVolumeCloneRestore_Success(t *testing.T) {
	hs := newReadyVolumeCloneRestoreStore()
	down := &fakeDownloader{content: "tar-bytes"}
	restorer := &fakeVolumeRestorer{}
	volumes := &fakeVolumeCreator{}
	fixed := time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)
	r := &VolumeCloneRestoreRunner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		Downloader:     down,
		VolumeRestorer: restorer,
		Volumes:        volumes,
		Now:            func() time.Time { return fixed },
	}

	err := r.RunVolumeCloneRestore(context.Background(), "vcr_1", "web", "data", "clone-web-data-abc123", "bkh_1")
	if err != nil {
		t.Fatalf("RunVolumeCloneRestore() error = %v", err)
	}

	if len(volumes.created) != 1 || volumes.created[0] != "clone-web-data-abc123" {
		t.Fatalf("EnsureVolume calls = %+v, want exactly one call for clone-web-data-abc123", volumes.created)
	}
	if len(hs.started) != 1 || hs.started[0].ID != "vcr_1" || hs.started[0].SourceServiceName != "web" || hs.started[0].SourceVolumeName != "data" || hs.started[0].NewVolumeName != "clone-web-data-abc123" {
		t.Fatalf("StartVolumeCloneRestore call = %+v, want one call for vcr_1/web/data/clone-web-data-abc123", hs.started)
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusSucceeded {
		t.Fatalf("FinishVolumeCloneRestore calls = %+v, want one succeeded call", hs.finished)
	}
	if restorer.gotVolumeName != "clone-web-data-abc123" {
		t.Errorf("restorer received volume %q, want %q", restorer.gotVolumeName, "clone-web-data-abc123")
	}
	if restorer.gotBody != "tar-bytes" {
		t.Errorf("restored body = %q, want %q", restorer.gotBody, "tar-bytes")
	}
}

// TestVolumeCloneRestoreRunner_RunVolumeCloneRestore_VolumeCreationFails_RecordsFailure
// proves a failure to create the new volume never reaches the restorer:
// there is nothing to restore into yet.
func TestVolumeCloneRestoreRunner_RunVolumeCloneRestore_VolumeCreationFails_RecordsFailure(t *testing.T) {
	hs := newReadyVolumeCloneRestoreStore()
	restorer := &fakeVolumeRestorer{}
	r := &VolumeCloneRestoreRunner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		Downloader:     &fakeDownloader{content: "tar-bytes"},
		VolumeRestorer: restorer,
		Volumes:        &fakeVolumeCreator{err: errors.New("docker daemon unreachable")},
	}

	err := r.RunVolumeCloneRestore(context.Background(), "vcr_1", "web", "data", "clone-web-data-abc123", "bkh_1")
	if err == nil {
		t.Fatal("RunVolumeCloneRestore() error = nil, want the volume creation failure surfaced")
	}
	if restorer.gotVolumeName != "" {
		t.Error("VolumeRestorer.Restore was called despite the new volume never being created")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
}

func TestVolumeCloneRestoreRunner_RunVolumeCloneRestore_BackupNotSucceeded_RefusesAndRecordsFailure(t *testing.T) {
	backup := newTestVolumeBackup()
	backup.Status = store.BackupStatusFailed
	hs := newReadyVolumeCloneRestoreStore()
	hs.backups["bkh_1"] = backup
	restorer := &fakeVolumeRestorer{}
	volumes := &fakeVolumeCreator{}
	r := &VolumeCloneRestoreRunner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		Downloader:     &fakeDownloader{content: "tar-bytes"},
		VolumeRestorer: restorer,
		Volumes:        volumes,
	}

	err := r.RunVolumeCloneRestore(context.Background(), "vcr_1", "web", "data", "clone-web-data-abc123", "bkh_1")
	if err == nil {
		t.Fatal("RunVolumeCloneRestore() error = nil, want a refusal for a backup that never succeeded")
	}
	if restorer.gotVolumeName != "" {
		t.Error("VolumeRestorer.Restore was called for a backup that never succeeded, want it never invoked")
	}
	// The volume is still created before the backup status is checked
	// (EnsureVolume runs first in createAndRestore): a harmless no-op
	// against an empty volume, not a real cost worth guarding against with
	// extra ordering complexity.
	if len(volumes.created) != 1 {
		t.Errorf("EnsureVolume calls = %d, want 1", len(volumes.created))
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
}

func TestVolumeCloneRestoreRunner_RunVolumeCloneRestore_RestoreFails_RecordsFailure(t *testing.T) {
	hs := newReadyVolumeCloneRestoreStore()
	r := &VolumeCloneRestoreRunner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		Downloader:     &fakeDownloader{content: "tar-bytes"},
		VolumeRestorer: &fakeVolumeRestorer{err: errors.New("tar: corrupt archive")},
		Volumes:        &fakeVolumeCreator{},
	}

	err := r.RunVolumeCloneRestore(context.Background(), "vcr_1", "web", "data", "clone-web-data-abc123", "bkh_1")
	if err == nil {
		t.Fatal("RunVolumeCloneRestore() error = nil, want the restore failure surfaced")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
}
