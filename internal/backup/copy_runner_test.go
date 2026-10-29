package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type copyTestStore struct {
	settings store.BackupCopySettings
	claim    []store.BackupCopy
	finished map[string]string
}

func (f *copyTestStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	return store.BackupTarget{ID: id, Bucket: "b-" + id}, nil
}
func (f *copyTestStore) GetBackupCopySettings(context.Context) (store.BackupCopySettings, error) {
	return f.settings, nil
}
func (f *copyTestStore) EnqueueBackupCopies(context.Context, string, string, string) error {
	return nil
}
func (f *copyTestStore) ClaimBackupCopies(context.Context, int) ([]store.BackupCopy, error) {
	return f.claim, nil
}
func (f *copyTestStore) FinishBackupCopy(_ context.Context, id string, _ int64, errMsg, _ string) error {
	f.finished[id] = errMsg
	return nil
}
func (f *copyTestStore) ResetStuckBackupCopies(context.Context) error { return nil }

type copyTestSecrets struct{}

func (copyTestSecrets) Resolve(context.Context, string, string) (string, error) { return "x", nil }

type copyTestDownloader struct{}

func (copyTestDownloader) Download(_ context.Context, d Destination, key string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(d.Bucket + ":" + key)), nil
}

type copyTestUploader struct {
	got  map[string]string
	fail bool
}

func (u *copyTestUploader) Upload(_ context.Context, d Destination, key string, r io.Reader, _ int64) error {
	if u.fail {
		return errors.New("bucket unreachable")
	}
	var b bytes.Buffer
	_, _ = io.Copy(&b, r)
	u.got[d.Bucket+"/"+key] = b.String()
	return nil
}

func TestCopyRunnerCopiesToSecondDestination(t *testing.T) {
	fs := &copyTestStore{
		settings: store.BackupCopySettings{Enabled: true, TargetID: "b"},
		claim:    []store.BackupCopy{{BackupHistoryID: "h1", SourceTargetID: "a", TargetID: "b", ObjectKey: "k1"}},
		finished: map[string]string{},
	}
	up := &copyTestUploader{got: map[string]string{}}
	r := &CopyRunner{Store: fs, Secrets: copyTestSecrets{}, Downloader: copyTestDownloader{}, Uploader: up, Concurrency: 1}
	n, err := r.RunOnce(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("RunOnce = %d, %v", n, err)
	}
	if up.got["b-b/k1"] != "b-a:k1" || fs.finished["h1"] != "" {
		t.Fatalf("copy wrong: %v %v", up.got, fs.finished)
	}

	up.fail = true
	if n, _ = r.RunOnce(context.Background()); n != 0 || !strings.Contains(fs.finished["h1"], "second destination") {
		t.Fatalf("failure not recorded: %d %v", n, fs.finished)
	}
}

func TestCopyRunnerIdleWhenDisabled(t *testing.T) {
	fs := &copyTestStore{claim: []store.BackupCopy{{BackupHistoryID: "h1"}}, finished: map[string]string{}}
	r := &CopyRunner{Store: fs, Secrets: copyTestSecrets{}}
	if n, err := r.RunOnce(context.Background()); n != 0 || err != nil || len(fs.finished) != 0 {
		t.Fatalf("disabled should do nothing: %d %v", n, err)
	}
}
