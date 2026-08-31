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

	err := r.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test")
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

func TestRunner_RunVolumeBackup_ArchiveFailure_RecordsFailure(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	r := &Runner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		VolumeArchiver: &fakeVolumeArchiver{err: errors.New("archive failed")},
		Uploader:       &fakeUploader{},
	}

	err := r.RunVolumeBackup(context.Background(), "bkh_1", "web", "data", "app-web-data", "bkt_test")
	if err == nil {
		t.Fatal("RunVolumeBackup() error = nil, want the archive failure")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", hs.finished)
	}
}
