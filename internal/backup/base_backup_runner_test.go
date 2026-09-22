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

type fakeBaseBackupHistoryStore struct {
	targets  map[string]store.BackupTarget
	started  []store.BaseBackupHistory
	finished []struct {
		id, status, lsn, errMsg, finishedAt string
		sizeBytes                           int64
	}
	getTargetErr error
	startErr     error
	finishErr    error
}

func (f *fakeBaseBackupHistoryStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	if f.getTargetErr != nil {
		return store.BackupTarget{}, f.getTargetErr
	}
	t, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return t, nil
}

func (f *fakeBaseBackupHistoryStore) StartBaseBackupHistory(_ context.Context, h store.BaseBackupHistory) error {
	if f.startErr != nil {
		return f.startErr
	}
	f.started = append(f.started, h)
	return nil
}

func (f *fakeBaseBackupHistoryStore) FinishBaseBackupHistory(_ context.Context, id, status string, sizeBytes int64, lsn, errMsg, finishedAt string) error {
	if f.finishErr != nil {
		return f.finishErr
	}
	f.finished = append(f.finished, struct {
		id, status, lsn, errMsg, finishedAt string
		sizeBytes                           int64
	}{id, status, lsn, errMsg, finishedAt, sizeBytes})
	return nil
}

type fakeBaseBackuper struct {
	gotContainer string
	content      string
	err          error
}

func (f *fakeBaseBackuper) BaseBackup(_ context.Context, containerName string) (io.ReadCloser, error) {
	f.gotContainer = containerName
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.content)), nil
}

func newTestBaseBackupRunner(store *fakeBaseBackupHistoryStore, backuper *fakeBaseBackuper, uploader *fakeUploader) *BaseBackupRunner {
	return &BaseBackupRunner{
		Store:        store,
		Secrets:      stubSecrets{},
		BaseBackuper: backuper,
		Uploader:     uploader,
		Now:          func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) },
	}
}

type stubSecrets struct{}

func (stubSecrets) Resolve(_ context.Context, _, envKey string) (string, error) {
	return "secret-" + envKey, nil
}

func TestBaseBackupRunner_RunBaseBackup_Success(t *testing.T) {
	st := &fakeBaseBackupHistoryStore{targets: map[string]store.BackupTarget{
		"tgt_1": {ID: "tgt_1", Provider: store.BackupProviderCustom, Bucket: "b", Endpoint: "https://s3.example"},
	}}
	backuper := &fakeBaseBackuper{content: "fake-tar-bytes"}
	uploader := &fakeUploader{}
	r := newTestBaseBackupRunner(st, backuper, uploader)

	if err := r.RunBaseBackup(context.Background(), "bbh_1", "mydb", "db-mydb", "tgt_1"); err != nil {
		t.Fatalf("RunBaseBackup() error = %v", err)
	}

	if len(st.started) != 1 || st.started[0].ID != "bbh_1" || st.started[0].DatabaseName != "mydb" {
		t.Fatalf("started = %+v", st.started)
	}
	if len(st.finished) != 1 || st.finished[0].status != store.BackupStatusSucceeded {
		t.Fatalf("finished = %+v", st.finished)
	}
	if st.finished[0].sizeBytes != int64(len("fake-tar-bytes")) {
		t.Errorf("sizeBytes = %d, want %d", st.finished[0].sizeBytes, len("fake-tar-bytes"))
	}
	if backuper.gotContainer != "db-mydb" {
		t.Errorf("BaseBackup called with container %q, want db-mydb", backuper.gotContainer)
	}
	if uploader.gotBody != "fake-tar-bytes" {
		t.Errorf("uploaded body = %q, want fake-tar-bytes", uploader.gotBody)
	}
}

func TestBaseBackupRunner_RunBaseBackup_BaseBackupFails_RecordsFailure(t *testing.T) {
	st := &fakeBaseBackupHistoryStore{targets: map[string]store.BackupTarget{
		"tgt_1": {ID: "tgt_1", Provider: store.BackupProviderCustom, Bucket: "b", Endpoint: "https://s3.example"},
	}}
	backuper := &fakeBaseBackuper{err: errors.New("pg_basebackup exited 1")}
	uploader := &fakeUploader{}
	r := newTestBaseBackupRunner(st, backuper, uploader)

	err := r.RunBaseBackup(context.Background(), "bbh_1", "mydb", "db-mydb", "tgt_1")
	if err == nil {
		t.Fatal("RunBaseBackup() error = nil, want the base backup failure")
	}
	if len(st.finished) != 1 || st.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", st.finished)
	}
}

func TestBaseBackupRunner_RunBaseBackup_UploadFails_RecordsFailure(t *testing.T) {
	st := &fakeBaseBackupHistoryStore{targets: map[string]store.BackupTarget{
		"tgt_1": {ID: "tgt_1", Provider: store.BackupProviderCustom, Bucket: "b", Endpoint: "https://s3.example"},
	}}
	backuper := &fakeBaseBackuper{content: "tar"}
	uploader := &fakeUploader{err: errors.New("bucket unreachable")}
	r := newTestBaseBackupRunner(st, backuper, uploader)

	err := r.RunBaseBackup(context.Background(), "bbh_1", "mydb", "db-mydb", "tgt_1")
	if err == nil {
		t.Fatal("RunBaseBackup() error = nil, want the upload failure")
	}
	if len(st.finished) != 1 || st.finished[0].status != store.BackupStatusFailed {
		t.Fatalf("finished = %+v, want one failed row", st.finished)
	}
}

func TestBaseBackupRunner_RunBaseBackup_UnknownTarget_RecordsFailure(t *testing.T) {
	st := &fakeBaseBackupHistoryStore{targets: map[string]store.BackupTarget{}}
	backuper := &fakeBaseBackuper{content: "tar"}
	uploader := &fakeUploader{}
	r := newTestBaseBackupRunner(st, backuper, uploader)

	err := r.RunBaseBackup(context.Background(), "bbh_1", "mydb", "db-mydb", "missing")
	if err == nil {
		t.Fatal("RunBaseBackup() error = nil, want the target lookup failure")
	}
	if backuper.gotContainer != "" {
		t.Error("BaseBackup should never have been called: target resolution failed first")
	}
}
