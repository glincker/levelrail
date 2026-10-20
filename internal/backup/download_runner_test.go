package backup

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeDownloadHistoryStore struct {
	backups      map[string]store.BackupHistory
	targets      map[string]store.BackupTarget
	getBackupErr error
	getTargetErr error
}

func (f *fakeDownloadHistoryStore) GetBackupHistory(_ context.Context, id string) (store.BackupHistory, error) {
	if f.getBackupErr != nil {
		return store.BackupHistory{}, f.getBackupErr
	}
	b, ok := f.backups[id]
	if !ok {
		return store.BackupHistory{}, store.ErrBackupHistoryNotFound
	}
	return b, nil
}

func (f *fakeDownloadHistoryStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	if f.getTargetErr != nil {
		return store.BackupTarget{}, f.getTargetErr
	}
	t, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return t, nil
}

func TestDownloadRunner_Download_Success(t *testing.T) {
	hs := &fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	down := &fakeDownloader{content: "dump-bytes"}
	d := &DownloadRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: down,
	}

	stream, err := d.Download(context.Background(), "bkh_1")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer func() {
		_ = stream.Close()
	}()

	body, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if string(body) != "dump-bytes" {
		t.Errorf("body = %q, want %q", body, "dump-bytes")
	}
	if down.gotKey != "mydb/mydb-20260814T030000Z.dump" {
		t.Errorf("download key = %q, want the backup's own object key", down.gotKey)
	}
	if down.gotDest.AccessKeyID != "AKIATEST" || down.gotDest.SecretAccessKey != "shh" {
		t.Errorf("download dest credentials = %+v, want resolved secrets", down.gotDest)
	}
}

func TestDownloadRunner_Download_BackupNotFound(t *testing.T) {
	hs := &fakeDownloadHistoryStore{backups: map[string]store.BackupHistory{}}
	d := &DownloadRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{},
	}

	_, err := d.Download(context.Background(), "bkh_missing")
	if err == nil {
		t.Fatal("Download() error = nil, want an error for a missing backup")
	}
}

// TestDownloadRunner_Download_NotSucceeded_RefusesWithoutContactingBucket
// is the core safety property Download's own doc comment describes: an
// attempt that never actually succeeded (still running, or already
// failed) is refused outright, before any credential resolution or
// bucket call happens, regardless of what internal/api's own synchronous
// check already did before calling this.
func TestDownloadRunner_Download_NotSucceeded_RefusesWithoutContactingBucket(t *testing.T) {
	for _, status := range []string{store.BackupStatusRunning, store.BackupStatusFailed} {
		t.Run(status, func(t *testing.T) {
			backup := newTestBackup()
			backup.Status = status
			hs := &fakeDownloadHistoryStore{
				backups: map[string]store.BackupHistory{"bkh_1": backup},
				targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
			}
			down := &fakeDownloader{content: "dump-bytes"}
			d := &DownloadRunner{
				Store:      hs,
				Secrets:    newTestSecrets(),
				Downloader: down,
			}

			_, err := d.Download(context.Background(), "bkh_1")
			if err == nil {
				t.Fatalf("Download() error = nil, want a refusal for a backup with status %q", status)
			}
			if down.gotKey != "" {
				t.Errorf("Downloader was called (key = %q) for a backup with status %q, want it never invoked", down.gotKey, status)
			}
		})
	}
}

func TestDownloadRunner_Download_TargetNotFound(t *testing.T) {
	hs := &fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{},
	}
	d := &DownloadRunner{
		Store:      hs,
		Secrets:    newTestSecrets(),
		Downloader: &fakeDownloader{content: "dump-bytes"},
	}

	_, err := d.Download(context.Background(), "bkh_1")
	if err == nil {
		t.Fatal("Download() error = nil, want an error for a missing target")
	}
}

func TestDownloadRunner_Download_SecretResolveFails_NeverContactsBucket(t *testing.T) {
	hs := &fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}
	down := &fakeDownloader{content: "dump-bytes"}
	d := &DownloadRunner{
		Store:      hs,
		Secrets:    &fakeSecrets{err: errors.New("secrets: master key not configured")},
		Downloader: down,
	}

	_, err := d.Download(context.Background(), "bkh_1")
	if err == nil {
		t.Fatal("Download() error = nil, want the secret resolution failure surfaced")
	}
	if down.gotKey != "" {
		t.Error("Downloader was called despite a secret resolution failure, want it skipped")
	}
}
