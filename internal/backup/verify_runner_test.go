package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeVerificationHistoryStore struct {
	backups  map[string]store.BackupHistory
	targets  map[string]store.BackupTarget
	started  []store.BackupVerification
	finished []struct {
		id, status, errMsg, finishedAt        string
		checksumMatch, sizeMatch, formatValid bool
		downloadedBytes                       int64
	}
	getBackupErr error
	getTargetErr error
	startErr     error
	finishErr    error
}

func (f *fakeVerificationHistoryStore) GetBackupHistory(_ context.Context, id string) (store.BackupHistory, error) {
	if f.getBackupErr != nil {
		return store.BackupHistory{}, f.getBackupErr
	}
	b, ok := f.backups[id]
	if !ok {
		return store.BackupHistory{}, store.ErrBackupHistoryNotFound
	}
	return b, nil
}

func (f *fakeVerificationHistoryStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	if f.getTargetErr != nil {
		return store.BackupTarget{}, f.getTargetErr
	}
	t, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return t, nil
}

func (f *fakeVerificationHistoryStore) StartBackupVerification(_ context.Context, v store.BackupVerification) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.started = append(f.started, v)
	return nil
}

func (f *fakeVerificationHistoryStore) FinishBackupVerification(_ context.Context, id, status string, checksumMatch, sizeMatch, formatValid bool, downloadedBytes int64, errMsg, finishedAt string) error {
	if f.finishErr != nil {
		return f.finishErr
	}
	f.finished = append(f.finished, struct {
		id, status, errMsg, finishedAt        string
		checksumMatch, sizeMatch, formatValid bool
		downloadedBytes                       int64
	}{id, status, errMsg, finishedAt, checksumMatch, sizeMatch, formatValid, downloadedBytes})
	return nil
}

// newTestSucceededBackup builds a store.BackupHistory whose SizeBytes and
// ChecksumSHA256 are the real size/hash of content, so verify_runner_test
// cases can control whether those checks should pass or fail just by
// choosing content, without hand-computing a checksum each time.
func newTestSucceededBackup(content string) store.BackupHistory {
	sum := sha256.Sum256([]byte(content))
	return store.BackupHistory{
		ID:             "bkh_1",
		DatabaseName:   "mydb",
		TargetID:       "bkt_test",
		ObjectKey:      "mydb/mydb-20260814T030000Z.dump",
		Status:         store.BackupStatusSucceeded,
		SizeBytes:      int64(len(content)),
		ChecksumSHA256: hex.EncodeToString(sum[:]),
	}
}

func TestVerifyRunner_VerifyBackup_Passed(t *testing.T) {
	backup := newTestSucceededBackup("dump-bytes")
	hs := &fakeVerificationHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": backup},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	down := &fakeDownloader{content: "dump-bytes"}
	fixed := time.Date(2026, 8, 15, 4, 0, 0, 0, time.UTC)
	vr := &VerifyRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: down,
		Now:        func() time.Time { return fixed },
	}

	err := vr.VerifyBackup(context.Background(), "bkv_1", "bkh_1", store.EnginePostgres, "alice")
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}

	if len(hs.started) != 1 || hs.started[0].ID != "bkv_1" || hs.started[0].BackupHistoryID != "bkh_1" || hs.started[0].CheckedBy != "alice" {
		t.Fatalf("StartBackupVerification call = %+v, want one call for bkv_1/bkh_1/alice", hs.started)
	}
	if len(hs.finished) != 1 {
		t.Fatalf("FinishBackupVerification calls = %d, want 1", len(hs.finished))
	}
	got := hs.finished[0]
	if got.status != store.BackupVerificationStatusPassed {
		t.Errorf("finish status = %q, want %q", got.status, store.BackupVerificationStatusPassed)
	}
	if !got.checksumMatch || !got.sizeMatch || !got.formatValid {
		t.Errorf("finish checks = %+v, want all true", got)
	}
	if got.downloadedBytes != int64(len("dump-bytes")) {
		t.Errorf("finish downloadedBytes = %d, want %d", got.downloadedBytes, len("dump-bytes"))
	}
	if got.errMsg != "" {
		t.Errorf("finish errMsg = %q, want empty", got.errMsg)
	}
	if down.gotKey != "mydb/mydb-20260814T030000Z.dump" {
		t.Errorf("download key = %q, want the backup's own object key", down.gotKey)
	}
}

func TestVerifyRunner_VerifyBackup_ChecksumMismatch_Fails(t *testing.T) {
	backup := newTestSucceededBackup("dump-bytes")
	backup.ChecksumSHA256 = "not-the-real-checksum"
	hs := &fakeVerificationHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": backup},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	vr := &VerifyRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{content: "dump-bytes"},
	}

	err := vr.VerifyBackup(context.Background(), "bkv_1", "bkh_1", store.EnginePostgres, "alice")
	if err == nil {
		t.Fatal("VerifyBackup() error = nil, want a checksum-mismatch failure")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupVerificationStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupVerificationStatusFailed", hs.finished)
	}
	if hs.finished[0].checksumMatch {
		t.Error("finish checksumMatch = true, want false")
	}
}

func TestVerifyRunner_VerifyBackup_SizeMismatch_Fails(t *testing.T) {
	backup := newTestSucceededBackup("dump-bytes")
	backup.SizeBytes = 99999
	hs := &fakeVerificationHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": backup},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	vr := &VerifyRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{content: "dump-bytes"},
	}

	err := vr.VerifyBackup(context.Background(), "bkv_1", "bkh_1", store.EnginePostgres, "alice")
	if err == nil {
		t.Fatal("VerifyBackup() error = nil, want a size-mismatch failure")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupVerificationStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupVerificationStatusFailed", hs.finished)
	}
	if hs.finished[0].sizeMatch {
		t.Error("finish sizeMatch = true, want false")
	}
}

func TestVerifyRunner_VerifyBackup_RedisMagicMismatch_Fails(t *testing.T) {
	backup := newTestSucceededBackup("not-an-rdb-file")
	hs := &fakeVerificationHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": backup},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	vr := &VerifyRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{content: "not-an-rdb-file"},
	}

	err := vr.VerifyBackup(context.Background(), "bkv_1", "bkh_1", store.EngineRedis, "alice")
	if err == nil {
		t.Fatal("VerifyBackup() error = nil, want an RDB-magic-mismatch failure")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupVerificationStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupVerificationStatusFailed", hs.finished)
	}
	if hs.finished[0].formatValid {
		t.Error("finish formatValid = true, want false")
	}
}

func TestVerifyRunner_VerifyBackup_RedisMagicPresent_Passes(t *testing.T) {
	content := "REDIS0011" + "restofthefile"
	backup := newTestSucceededBackup(content)
	hs := &fakeVerificationHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": backup},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	vr := &VerifyRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{content: content},
	}

	err := vr.VerifyBackup(context.Background(), "bkv_1", "bkh_1", store.EngineRedis, "alice")
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v", err)
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupVerificationStatusPassed {
		t.Fatalf("finish calls = %+v, want exactly one BackupVerificationStatusPassed", hs.finished)
	}
}

// TestVerifyRunner_VerifyBackup_BackupNotSucceeded_RefusesAndRecordsFailure
// mirrors RestoreRunner's own identical safety property
// (restore_runner_test.go): verifying a backup that never actually
// succeeded is refused outright, never attempted.
func TestVerifyRunner_VerifyBackup_BackupNotSucceeded_RefusesAndRecordsFailure(t *testing.T) {
	for _, status := range []string{store.BackupStatusRunning, store.BackupStatusFailed} {
		t.Run(status, func(t *testing.T) {
			backup := newTestSucceededBackup("dump-bytes")
			backup.Status = status
			hs := &fakeVerificationHistoryStore{
				backups: map[string]store.BackupHistory{"bkh_1": backup},
				targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
			}
			down := &fakeDownloader{content: "dump-bytes"}
			vr := &VerifyRunner{
				Store:      hs,
				Secrets:    newTestSecrets(),
				Downloader: down,
			}

			err := vr.VerifyBackup(context.Background(), "bkv_1", "bkh_1", store.EnginePostgres, "alice")
			if err == nil {
				t.Fatalf("VerifyBackup() error = nil, want a refusal for a backup with status %q", status)
			}
			if down.gotKey != "" {
				t.Errorf("Downloader was called (key = %q) for a backup with status %q, want it never invoked", down.gotKey, status)
			}
			if len(hs.finished) != 1 || hs.finished[0].status != store.BackupVerificationStatusFailed {
				t.Fatalf("finish calls = %+v, want exactly one BackupVerificationStatusFailed", hs.finished)
			}
			if hs.finished[0].errMsg == "" {
				t.Error("finish errMsg is empty, want the not-succeeded reason recorded")
			}
		})
	}
}

func TestVerifyRunner_VerifyBackup_DownloadFails_RecordsFailure(t *testing.T) {
	backup := newTestSucceededBackup("dump-bytes")
	hs := &fakeVerificationHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": backup},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	vr := &VerifyRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{err: errors.New("bucket: object not found")},
	}

	err := vr.VerifyBackup(context.Background(), "bkv_1", "bkh_1", store.EnginePostgres, "alice")
	if err == nil {
		t.Fatal("VerifyBackup() error = nil, want the download failure surfaced")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupVerificationStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupVerificationStatusFailed", hs.finished)
	}
}

func TestVerifyRunner_VerifyBackup_SecretResolveFails_NeverDownloads(t *testing.T) {
	backup := newTestSucceededBackup("dump-bytes")
	hs := &fakeVerificationHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": backup},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	down := &fakeDownloader{content: "dump-bytes"}
	vr := &VerifyRunner{
		Store:      hs,
		Secrets:    &fakeSecrets{err: errors.New("secrets: master key not configured")},
		Downloader: down,
	}

	err := vr.VerifyBackup(context.Background(), "bkv_1", "bkh_1", store.EnginePostgres, "alice")
	if err == nil {
		t.Fatal("VerifyBackup() error = nil, want the secret resolution failure surfaced")
	}
	if down.gotKey != "" {
		t.Error("Downloader was called despite a secret resolution failure, want it skipped")
	}
}

func TestVerifyRunner_VerifyBackup_LegacyBackupWithNoChecksumOrSize_ChecksOnlyFormat(t *testing.T) {
	backup := store.BackupHistory{
		ID: "bkh_1", DatabaseName: "mydb", TargetID: "bkt_test",
		ObjectKey: "mydb/mydb-1.dump", Status: store.BackupStatusSucceeded,
	}
	hs := &fakeVerificationHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": backup},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	vr := &VerifyRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{content: "dump-bytes"},
	}

	err := vr.VerifyBackup(context.Background(), "bkv_1", "bkh_1", store.EnginePostgres, "alice")
	if err != nil {
		t.Fatalf("VerifyBackup() error = %v, want a legacy row with no recorded checksum/size to still pass", err)
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupVerificationStatusPassed {
		t.Fatalf("finish calls = %+v, want exactly one BackupVerificationStatusPassed", hs.finished)
	}
}
