package backup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/GLINCKER/levelrail/internal/store"
)

type sealRecordingStore struct {
	*fakeHistoryStore
	codec string
}

func (s *sealRecordingStore) SetBackupSealInfo(_ context.Context, _, codec, _ string, _, _ int64) error {
	s.codec = codec
	return nil
}

func TestEncryptedDump_RoundTrip(t *testing.T) {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	sealer := &Sealer{Recipients: []age.Recipient{id.Recipient()}, Identities: []age.Identity{id}}
	target := newTestTarget()
	target.EncryptDumps = true
	hs := &sealRecordingStore{fakeHistoryStore: &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": target}}}
	up := &fakeUploader{}
	plaintext := "CREATE TABLE secrets (v text); -- PostgreSQL database dump complete"

	r := &Runner{Store: hs, Secrets: newTestSecrets(), Dumper: &fakeDumper{content: plaintext}, Uploader: up, Volume: &VolumeBackupOptions{Sealer: sealer}}
	if err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc", "bkt_test"); err != nil {
		t.Fatalf("RunBackup: %v", err)
	}
	if hs.codec != CodecZstdAge || !strings.HasSuffix(up.gotKey, ".dump.zst.age") {
		t.Fatalf("codec %q key %q: dump was not sealed", hs.codec, up.gotKey)
	}
	if strings.Contains(up.gotBody, "secrets") {
		t.Fatal("uploaded object contains plaintext")
	}

	bh := newTestBackup()
	bh.Codec, bh.ObjectKey = hs.codec, up.gotKey
	rs := &fakeRestoreHistoryStore{backups: map[string]store.BackupHistory{"bkh_1": bh}, targets: map[string]store.BackupTarget{"bkt_test": target}}
	restorer := &fakeRestorer{}
	rr := &RestoreRunner{Store: rs, Secrets: newTestSecrets(), Downloader: &fakeDownloader{content: up.gotBody}, Restorer: restorer, Identities: sealer.Identities}
	if err := rr.RunRestore(context.Background(), "rst_1", "mydb", "bkh_1", "postgres", "mydb-abc"); err != nil {
		t.Fatalf("RunRestore: %v", err)
	}
	if restorer.gotBody != plaintext {
		t.Fatalf("restored %q, want the original dump", restorer.gotBody)
	}
}

func TestEncryptedDump_FailsClosedWithoutKey(t *testing.T) {
	target := newTestTarget()
	target.EncryptDumps = true
	hs := &fakeHistoryStore{targets: map[string]store.BackupTarget{"bkt_test": target}}
	up := &fakeUploader{}
	r := &Runner{Store: hs, Secrets: newTestSecrets(), Dumper: &fakeDumper{content: "x"}, Uploader: up}
	err := r.RunBackup(context.Background(), "bkh_1", "mydb", "postgres", "mydb-abc", "bkt_test")
	if !errors.Is(err, ErrDumpEncryptionUnavailable) || up.gotKey != "" {
		t.Fatalf("err %v, uploaded key %q: want a refusal and no upload", err, up.gotKey)
	}
}
