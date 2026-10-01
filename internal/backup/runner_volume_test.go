package backup

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeVolumeArchiver struct {
	gotVolumeName string
	content       string
	err           error
}

func (f *fakeVolumeArchiver) Archive(_ context.Context, volumeName string) (io.ReadCloser, error) {
	f.gotVolumeName = volumeName
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), nil
}

func TestRunner_RunVolumeBackup_Success(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	up := &fakeUploader{}
	archiver := &fakeVolumeArchiver{content: "tar-bytes"}
	fixed := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	r := &Runner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		VolumeArchiver: archiver,
		Uploader:       up,
		Now:            func() time.Time { return fixed },
	}

	err := r.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	if err != nil {
		t.Fatalf("RunVolumeBackup() error = %v", err)
	}

	if archiver.gotVolumeName != "app-web-data" {
		t.Errorf("archiver received volume %q, want the real Docker volume name %q", archiver.gotVolumeName, "app-web-data")
	}

	if len(hs.started) != 1 {
		t.Fatalf("StartBackupHistory calls = %d, want 1", len(hs.started))
	}
	got := hs.started[0]
	if got.ResourceKind != store.BackupResourceKindVolume || got.ServiceName != "web" || got.VolumeName != "data" {
		t.Errorf("started history = %+v, want ResourceKind=volume ServiceName=web VolumeName=data", got)
	}
	if got.DatabaseName != "" {
		t.Errorf("started history DatabaseName = %q, want empty for a volume backup", got.DatabaseName)
	}
	if !strings.HasPrefix(got.ObjectKey, "volumes/web/data/") || !strings.HasSuffix(got.ObjectKey, ".tar") {
		t.Errorf("object key = %q, want volumes/web/data/<timestamp>.tar shape", got.ObjectKey)
	}

	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusSucceeded {
		t.Fatalf("finished = %+v, want one succeeded row", hs.finished)
	}
	if up.gotBody != "tar-bytes" {
		t.Errorf("uploaded body = %q, want %q", up.gotBody, "tar-bytes")
	}
}

type fakeSqliteSnapshotter struct {
	gotVolumeName, gotRelPath string
	content                   string
	err                       error
}

func (f *fakeSqliteSnapshotter) Snapshot(_ context.Context, volumeName, relPath string) (io.ReadCloser, error) {
	f.gotVolumeName, f.gotRelPath = volumeName, relPath
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), nil
}

func TestRunner_RunVolumeBackup_SqlitePath_UsesSnapshotterNotArchiver(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	up := &fakeUploader{}
	archiver := &fakeVolumeArchiver{content: "should-not-be-used"}
	snap := &fakeSqliteSnapshotter{content: "consistent-db-bytes"}
	r := &Runner{
		Store:             hs,
		Secrets:           newTestSecrets(),
		VolumeArchiver:    archiver,
		SqliteSnapshotter: snap,
		Uploader:          up,
	}

	err := r.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "app/data.db")
	if err != nil {
		t.Fatalf("RunVolumeBackup() error = %v", err)
	}

	if archiver.gotVolumeName != "" {
		t.Error("VolumeArchiver.Archive was called, want only SqliteSnapshotter.Snapshot when sqlitePath is set")
	}
	if snap.gotVolumeName != "app-web-data" || snap.gotRelPath != "app/data.db" {
		t.Errorf("snapshotter received (%q, %q), want (%q, %q)", snap.gotVolumeName, snap.gotRelPath, "app-web-data", "app/data.db")
	}
	if up.gotBody != "consistent-db-bytes" {
		t.Errorf("uploaded body = %q, want the snapshot's bytes, not an archive", up.gotBody)
	}

	got := hs.started[0]
	if !strings.HasSuffix(got.ObjectKey, ".db") {
		t.Errorf("object key = %q, want a .db suffix for a sqlite snapshot, not .tar", got.ObjectKey)
	}
}

func TestRunner_RunVolumeBackup_SqliteSnapshotFailure_RecordsFailure(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	r := &Runner{
		Store:             hs,
		Secrets:           newTestSecrets(),
		SqliteSnapshotter: &fakeSqliteSnapshotter{err: errors.New("database is locked")},
		Uploader:          &fakeUploader{},
	}

	err := r.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "app/data.db")
	if err == nil {
		t.Fatal("RunVolumeBackup() error = nil, want the snapshot failure")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", hs.finished)
	}
}

func TestRunner_RunVolumeBackup_ArchiveFailure_RecordsFailure(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	r := &Runner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		VolumeArchiver: &fakeVolumeArchiver{err: errors.New("archive failed")},
		Uploader:       &fakeUploader{},
	}

	err := r.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test", "")
	if err == nil {
		t.Fatal("RunVolumeBackup() error = nil, want the archive failure")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", hs.finished)
	}
}
