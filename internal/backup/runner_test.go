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

type fakeHistoryStore struct {
	targets  map[string]store.BackupTarget
	started  []store.BackupHistory
	finished []struct {
		id, status, errMsg, finishedAt string
		size                           int64
	}
	getErr    error
	startErr  error
	finishErr error
}

func (f *fakeHistoryStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	if f.getErr != nil {
		return store.BackupTarget{}, f.getErr
	}
	t, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return t, nil
}

func (f *fakeHistoryStore) StartBackupHistory(_ context.Context, h store.BackupHistory) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.started = append(f.started, h)
	return nil
}

func (f *fakeHistoryStore) FinishBackupHistory(_ context.Context, id, status string, sizeBytes int64, errMsg, finishedAt string) error {
	if f.finishErr != nil {
		return f.finishErr
	}
	f.finished = append(f.finished, struct {
		id, status, errMsg, finishedAt string
		size                           int64
	}{id, status, errMsg, finishedAt, sizeBytes})
	return nil
}

type fakeSecrets struct {
	values map[string]string
	err    error
}

func (f *fakeSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.values[serviceName+"/"+envKey]
	if !ok {
		return "", errors.New("fakeSecrets: no value for " + serviceName + "/" + envKey)
	}
	return v, nil
}

type fakeDumper struct {
	content string
	err     error
}

func (f *fakeDumper) Dump(_ context.Context, _, _ string) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), nil
}

type fakeUploader struct {
	gotDest Destination
	gotKey  string
	gotBody string
	err     error
}

func (f *fakeUploader) Upload(_ context.Context, dest Destination, key string, r io.Reader, _ int64) error {
	f.gotDest = dest
	f.gotKey = key
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	f.gotBody = buf.String()
	return f.err
}

func newTestTarget() store.BackupTarget {
	return store.BackupTarget{
		ID:       "bkt_test",
		Name:     "primary",
		Provider: store.BackupProviderR2,
		Endpoint: "https://example.r2.cloudflarestorage.com",
		Region:   "auto",
		Bucket:   "levelrail-backups",
	}
}

func newTestSecrets() *fakeSecrets {
	key := store.BackupTargetSecretsKey("bkt_test")
	return &fakeSecrets{values: map[string]string{
		key + "/access_key_id":     "AKIATEST",
		key + "/secret_access_key": "shh",
	}}
}

func TestRunner_RunBackup_Success(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	up := &fakeUploader{}
	fixed := time.Date(2026, 8, 14, 3, 0, 0, 0, time.UTC)
	r := &Runner{
		Store:    hs,
		Secrets:  newTestSecrets(),
		Dumper:   &fakeDumper{content: "dump-bytes"},
		Uploader: up,
		Now:      func() time.Time { return fixed },
	}

	err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc123", "bkt_test")
	if err != nil {
		t.Fatalf("RunBackup() error = %v", err)
	}

	if len(hs.started) != 1 || hs.started[0].ID != "bkh_1" || hs.started[0].DatabaseName != "mydb" || hs.started[0].TargetID != "bkt_test" {
		t.Fatalf("StartBackupHistory call = %+v, want one call for bkh_1/mydb/bkt_test", hs.started)
	}
	if len(hs.finished) != 1 {
		t.Fatalf("FinishBackupHistory calls = %d, want 1", len(hs.finished))
	}
	got := hs.finished[0]
	if got.status != store.BackupStatusSucceeded {
		t.Errorf("finish status = %q, want %q", got.status, store.BackupStatusSucceeded)
	}
	if got.size != int64(len("dump-bytes")) {
		t.Errorf("finish size = %d, want %d", got.size, len("dump-bytes"))
	}
	if got.errMsg != "" {
		t.Errorf("finish errMsg = %q, want empty", got.errMsg)
	}

	if up.gotBody != "dump-bytes" {
		t.Errorf("uploaded body = %q, want %q", up.gotBody, "dump-bytes")
	}
	if up.gotDest.AccessKeyID != "AKIATEST" || up.gotDest.SecretAccessKey != "shh" {
		t.Errorf("uploaded dest credentials = %+v, want resolved secrets", up.gotDest)
	}
	if up.gotDest.Bucket != "levelrail-backups" || up.gotDest.Endpoint != "https://example.r2.cloudflarestorage.com" {
		t.Errorf("uploaded dest = %+v, want target's bucket/endpoint", up.gotDest)
	}
	if !strings.HasPrefix(up.gotKey, "mydb/mydb-") || !strings.HasSuffix(up.gotKey, ".dump") {
		t.Errorf("object key = %q, want mydb/mydb-<timestamp>.dump shape", up.gotKey)
	}
}

func TestRunner_RunBackup_TargetNotFound_RecordsFailure(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{}}
	r := &Runner{
		Store:    hs,
		Secrets:  newTestSecrets(),
		Dumper:   &fakeDumper{content: "x"},
		Uploader: &fakeUploader{},
	}

	err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc123", "bkt_missing")
	if err == nil {
		t.Fatal("RunBackup() error = nil, want an error for a missing target")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
	if hs.finished[0].errMsg == "" {
		t.Error("finish errMsg is empty, want the target-not-found reason recorded")
	}
}

func TestRunner_RunBackup_DumpFails_RecordsFailure_NeverUploads(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	up := &fakeUploader{}
	r := &Runner{
		Store:    hs,
		Secrets:  newTestSecrets(),
		Dumper:   &fakeDumper{err: errors.New("pg_dump: connection refused")},
		Uploader: up,
	}

	err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc123", "bkt_test")
	if err == nil {
		t.Fatal("RunBackup() error = nil, want the dump failure surfaced")
	}
	if up.gotKey != "" {
		t.Errorf("Uploader was called (key = %q) after a dump failure, want it never invoked", up.gotKey)
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
}

func TestRunner_RunBackup_UploadFails_RecordsFailureWithBytesSeen(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	r := &Runner{
		Store:    hs,
		Secrets:  newTestSecrets(),
		Dumper:   &fakeDumper{content: "twelve-bytes"},
		Uploader: &fakeUploader{err: errors.New("bucket: access denied")},
	}

	err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc123", "bkt_test")
	if err == nil {
		t.Fatal("RunBackup() error = nil, want the upload failure surfaced")
	}
	if len(hs.finished) != 1 || hs.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finish calls = %+v, want exactly one BackupStatusFailed", hs.finished)
	}
	if hs.finished[0].size != int64(len("twelve-bytes")) {
		t.Errorf("finish size = %d, want %d (bytes read before the upload failed)", hs.finished[0].size, len("twelve-bytes"))
	}
}

func TestRunner_RunBackup_SecretResolveFails_NeverDumps(t *testing.T) {
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()}}
	dumpCalled := false
	r := &Runner{
		Store:   hs,
		Secrets: &fakeSecrets{err: errors.New("secrets: master key not configured")},
		Dumper: dumperFunc(func(context.Context, string, string) (io.ReadCloser, error) {
			dumpCalled = true
			return io.NopCloser(strings.NewReader("")), nil
		}),
		Uploader: &fakeUploader{},
	}

	err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc123", "bkt_test")
	if err == nil {
		t.Fatal("RunBackup() error = nil, want the secret resolution failure surfaced")
	}
	if dumpCalled {
		t.Error("Dumper was called despite a secret resolution failure, want it skipped")
	}
}

type dumperFunc func(ctx context.Context, engine, containerName string) (io.ReadCloser, error)

func (f dumperFunc) Dump(ctx context.Context, engine, containerName string) (io.ReadCloser, error) {
	return f(ctx, engine, containerName)
}
