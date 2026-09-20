package backup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// withFakeDiskFree substitutes diskFreeFunc for the duration of a test,
// restoring the real diskspace.Free implementation afterward.
func withFakeDiskFree(t *testing.T, free map[string]int64) {
	t.Helper()
	orig := diskFreeFunc
	t.Cleanup(func() { diskFreeFunc = orig })
	diskFreeFunc = func(path string) (int64, error) {
		v, ok := free[path]
		if !ok {
			return 0, errors.New("diskspace: no such path")
		}
		return v, nil
	}
}

func TestCheckDiskSpace(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")

	tests := []struct {
		name    string
		free    map[string]int64
		dir     string
		wantErr bool
	}{
		{name: "above threshold allows", free: map[string]int64{"/data": 1 << 30}, dir: "/data", wantErr: false},
		{name: "below threshold blocks", free: map[string]int64{"/data": 10 << 20}, dir: "/data", wantErr: true},
		{name: "empty dir skips the check entirely", free: map[string]int64{}, dir: "", wantErr: false},
		{name: "unreadable dir degrades to allow, not block", free: map[string]int64{}, dir: "/does/not/exist", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withFakeDiskFree(t, tt.free)
			err := checkDiskSpace(tt.dir)
			if tt.wantErr && err == nil {
				t.Fatal("checkDiskSpace(): want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("checkDiskSpace(): want no error, got %v", err)
			}
		})
	}
}

func TestMinDiskSpaceBytes(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want int64
	}{
		{name: "unset uses default", env: "", want: defaultMinDiskSpaceMB << 20},
		{name: "valid override", env: "1024", want: 1024 << 20},
		{name: "invalid falls back to default", env: "not-a-number", want: defaultMinDiskSpaceMB << 20},
		{name: "zero falls back to default", env: "0", want: defaultMinDiskSpaceMB << 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envMinDiskSpaceMB, tt.env)
			if got := minDiskSpaceBytes(); got != tt.want {
				t.Fatalf("minDiskSpaceBytes() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestResolveWorkDir(t *testing.T) {
	t.Run("explicit wins over env", func(t *testing.T) {
		t.Setenv("APP_DATA_DIR", "/env/data")
		if got := resolveWorkDir("/explicit"); got != "/explicit" {
			t.Fatalf("resolveWorkDir() = %q, want %q", got, "/explicit")
		}
	})
	t.Run("falls back to APP_DATA_DIR", func(t *testing.T) {
		t.Setenv("APP_DATA_DIR", "/env/data")
		if got := resolveWorkDir(""); got != "/env/data" {
			t.Fatalf("resolveWorkDir() = %q, want %q", got, "/env/data")
		}
	})
	t.Run("empty when neither is set", func(t *testing.T) {
		t.Setenv("APP_DATA_DIR", "")
		if got := resolveWorkDir(""); got != "" {
			t.Fatalf("resolveWorkDir() = %q, want empty", got)
		}
	})
}

func TestRunner_RunBackup_InsufficientDiskSpace_NeverStartsHistory(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	withFakeDiskFree(t, map[string]int64{"/data": 1 << 20})

	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	r := &Runner{
		Store:    hs,
		Secrets:  newTestSecrets(),
		Dumper:   &fakeDumper{content: "dump-bytes"},
		Uploader: &fakeUploader{},
		WorkDir:  "/data",
	}

	err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc123", "bkt_test")
	if err == nil {
		t.Fatal("RunBackup() error = nil, want an error for insufficient disk space")
	}
	if len(hs.started) != 0 {
		t.Errorf("StartBackupHistory calls = %d, want 0: a disk-space preflight failure must fail before any history bookkeeping starts", len(hs.started))
	}
	if len(hs.finished) != 0 {
		t.Errorf("FinishBackupHistory calls = %d, want 0", len(hs.finished))
	}
}

func TestRunner_RunVolumeBackup_InsufficientDiskSpace_NeverStartsHistory(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	withFakeDiskFree(t, map[string]int64{"/data": 1 << 20})

	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	r := &Runner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		VolumeArchiver: &fakeVolumeArchiver{content: "tar-bytes"},
		Uploader:       &fakeUploader{},
		WorkDir:        "/data",
	}

	err := r.RunVolumeBackup(context.Background(), "bkh_1", "myapp", "data", "myapp_data", "bkt_test")
	if err == nil {
		t.Fatal("RunVolumeBackup() error = nil, want an error for insufficient disk space")
	}
	if len(hs.started) != 0 {
		t.Errorf("StartBackupHistory calls = %d, want 0", len(hs.started))
	}
}

func TestRunner_RunBackup_SufficientDiskSpace_Proceeds(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	withFakeDiskFree(t, map[string]int64{"/data": 1 << 30})

	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	fixed := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	r := &Runner{
		Store:    hs,
		Secrets:  newTestSecrets(),
		Dumper:   &fakeDumper{content: "dump-bytes"},
		Uploader: &fakeUploader{},
		WorkDir:  "/data",
		Now:      func() time.Time { return fixed },
	}

	if err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc123", "bkt_test"); err != nil {
		t.Fatalf("RunBackup() error = %v, want nil once disk space is sufficient", err)
	}
	if len(hs.started) != 1 {
		t.Fatalf("StartBackupHistory calls = %d, want 1", len(hs.started))
	}
}

func TestRestoreRunner_RunRestore_InsufficientDiskSpace_NeverStartsHistory(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	withFakeDiskFree(t, map[string]int64{"/data": 1 << 20})

	hs := &fakeRestoreHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	r := &RestoreRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{content: "dump-bytes"},
		Restorer:   &fakeRestorer{},
		WorkDir:    "/data",
	}

	err := r.RunRestore(context.Background(), "rh_1", "mydb", "bkh_1", "postgres", "mydb-abc123")
	if err == nil {
		t.Fatal("RunRestore() error = nil, want an error for insufficient disk space")
	}
	if len(hs.started) != 0 {
		t.Errorf("StartRestoreHistory calls = %d, want 0: a disk-space preflight failure must fail before any history bookkeeping starts", len(hs.started))
	}
}

func TestRestoreRunner_RunVolumeRestore_InsufficientDiskSpace_NeverStartsHistory(t *testing.T) {
	t.Setenv(envMinDiskSpaceMB, "")
	withFakeDiskFree(t, map[string]int64{"/data": 1 << 20})

	hs := &fakeRestoreHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	r := &RestoreRunner{
		Store:          hs,
		Secrets:        newTestSecrets(),
		Downloader:     &fakeDownloader{content: "tar-bytes"},
		VolumeRestorer: &fakeVolumeRestorer{},
		WorkDir:        "/data",
	}

	err := r.RunVolumeRestore(context.Background(), "rh_1", "myapp", "data", "myapp_data", "bkh_1")
	if err == nil {
		t.Fatal("RunVolumeRestore() error = nil, want an error for insufficient disk space")
	}
	if len(hs.started) != 0 {
		t.Errorf("StartRestoreHistory calls = %d, want 0", len(hs.started))
	}
}
