package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeVolumeRestorer struct {
	gotVolumeName string
	gotBody       string
	err           error
}

func (f *fakeVolumeRestorer) Restore(_ context.Context, volumeName string, archive io.Reader) error {
	f.gotVolumeName = volumeName
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, archive)
	f.gotBody = buf.String()
	return f.err
}

func newTestVolumeBackup() store.BackupHistory {
	return store.BackupHistory{
		ID:           "bkh_1",
		ResourceKind: store.BackupResourceKindVolume,
		ServiceName:  "web",
		VolumeName:   "data",
		TargetID:     "bkt_test",
		ObjectKey:    "volumes/web/data/20260814T030000Z.tar",
		Status:       store.BackupStatusSucceeded,
	}
}

func TestRestoreRunner_RunVolumeRestore_Success(t *testing.T) {
	hs := &fakeRestoreHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestVolumeBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	down := &fakeDownloader{content: "tar-bytes"}
	restorer := &fakeVolumeRestorer{}
	fixed := time.Date(2026, 8, 14, 4, 0, 0, 0, time.UTC)
	rr := &RestoreRunner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		Downloader:     down,
		VolumeRestorer: restorer,
		Now:            func() time.Time { return fixed },
	}

	err := rr.RunVolumeRestore(context.Background(), "rsh_1", "web", "data", "app-web-data", "bkh_1")
	if err != nil {
		t.Fatalf("RunVolumeRestore() error = %v", err)
	}

	if restorer.gotVolumeName != "app-web-data" {
		t.Errorf("restorer received volume %q, want %q", restorer.gotVolumeName, "app-web-data")
	}
	if restorer.gotBody != "tar-bytes" {
		t.Errorf("restorer received body %q, want %q", restorer.gotBody, "tar-bytes")
	}
	if len(hs.started) != 1 {
		t.Fatalf("StartRestoreHistory calls = %d, want 1", len(hs.started))
	}
	got := hs.started[0]
	if got.ResourceKind != store.BackupResourceKindVolume || got.ServiceName != "web" || got.VolumeName != "data" {
		t.Errorf("started restore history = %+v, want ResourceKind=volume ServiceName=web VolumeName=data", got)
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusSucceeded {
		t.Fatalf("finished = %+v, want one succeeded row", hs.finished)
	}
}

func TestRestoreRunner_RunVolumeRestore_RefusesUnsuccessfulBackup(t *testing.T) {
	backup := newTestVolumeBackup()
	backup.Status = store.BackupStatusFailed
	hs := &fakeRestoreHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": backup},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	restorer := &fakeVolumeRestorer{}
	rr := &RestoreRunner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		Downloader:     &fakeDownloader{content: "tar-bytes"},
		VolumeRestorer: restorer,
	}

	err := rr.RunVolumeRestore(context.Background(), "rsh_1", "web", "data", "app-web-data", "bkh_1")
	if err == nil {
		t.Fatal("RunVolumeRestore() error = nil, want a refusal since the named backup did not succeed")
	}
	if restorer.gotVolumeName != "" {
		t.Error("VolumeRestorer.Restore was called despite the backup not having succeeded")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", hs.finished)
	}
}

func TestRestoreRunner_RunVolumeRestore_RestoreFailure_RecordsFailure(t *testing.T) {
	hs := &fakeRestoreHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestVolumeBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	rr := &RestoreRunner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		Downloader:     &fakeDownloader{content: "tar-bytes"},
		VolumeRestorer: &fakeVolumeRestorer{err: errors.New("restore failed")},
	}

	err := rr.RunVolumeRestore(context.Background(), "rsh_1", "web", "data", "app-web-data", "bkh_1")
	if err == nil {
		t.Fatal("RunVolumeRestore() error = nil, want the restore failure")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", hs.finished)
	}
}
