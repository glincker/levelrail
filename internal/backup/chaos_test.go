package backup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestChaos_DiskFullDuringBackup_PreflightRefusesBeforeAnyWork(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	withFakeDiskFree(t, map[string]int64{"/data": 1 << 20})

	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	up := &fakeUploader{}
	r := &Runner{Store: hs, Secrets: newTestSecrets(), Dumper: &fakeDumper{content: "x"}, Uploader: up, WorkDir: "/data"}

	err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc123", "bkt_test")
	if err == nil || !strings.Contains(err.Error(), "insufficient disk space") {
		t.Fatalf("RunBackup() error = %v, want the stable insufficient disk space error", err)
	}
	if up.gotKey != "" || len(hs.started) != 0 {
		t.Errorf("work started despite a full disk: uploadKey=%q started=%d", up.gotKey, len(hs.started))
	}

	withFakeDiskFree(t, map[string]int64{"/data": 1 << 30})
	if err := r.RunBackup(context.Background(), "bkh_2", "mydb", "postgres", "mydb-abc123", "bkt_test"); err != nil {
		t.Fatalf("RunBackup() after space was freed error = %v", err)
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusSucceeded {
		t.Fatalf("finish calls = %+v, want one succeeded", hs.finished)
	}
}

func TestChaos_DiskFullDuringBackup_MidUploadRecordsFailedRow(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	r := &Runner{
		Store:    hs,
		Secrets:  newTestSecrets(),
		Dumper:   &fakeDumper{content: "partial"},
		Uploader: &fakeUploader{err: errors.New("write: no space left on device")},
	}

	err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc123", "bkt_test")
	if err == nil {
		t.Fatal("RunBackup() error = nil, want the ENOSPC failure surfaced")
	}
	if len(hs.finished) != 1 {
		t.Fatalf("finish calls = %d, want exactly 1", len(hs.finished))
	}
	got := hs.finished[0]
	if got.status != store.BackupStatusFailed || got.checksum != "" || !strings.Contains(got.errMsg, "no space left on device") {
		t.Errorf("finished = %+v, want failed, no checksum, ENOSPC message visible", got)
	}
}
