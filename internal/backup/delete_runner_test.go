package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeDeleteHistoryStore reuses fakeDownloadHistoryStore's exact shape
// plus a DeleteBackupHistory call log, since DeleteRunner needs the
// identical GetBackupHistory/GetBackupTarget pair DownloadRunner does.
type fakeDeleteHistoryStore struct {
	fakeDownloadHistoryStore
	deleted        []string
	deleteErr      error
	deleteNotFound bool
}

func (f *fakeDeleteHistoryStore) DeleteBackupHistory(_ context.Context, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	if f.deleteNotFound {
		return store.ErrBackupHistoryNotFound
	}
	f.deleted = append(f.deleted, id)
	return nil
}

func TestDeleteRunner_DeleteBackup_Success(t *testing.T) {
	hs := &fakeDeleteHistoryStore{fakeDownloadHistoryStore: fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}}
	del := &fakeDeleter{}
	d := &DeleteRunner{Store: hs, Secrets: newTestSecrets(), Deleter: del}

	if err := d.DeleteBackup(context.Background(), "bkh_1"); err != nil {
		t.Fatalf("DeleteBackup() error = %v", err)
	}

	if len(del.calls) != 1 {
		t.Fatalf("Deleter.Delete calls = %d, want 1", len(del.calls))
	}
	if del.calls[0].key != "mydb/mydb-20260814T030000Z.dump" {
		t.Errorf("delete key = %q, want the backup's own object key", del.calls[0].key)
	}
	if del.calls[0].dest.AccessKeyID != "AKIATEST" || del.calls[0].dest.SecretAccessKey != "shh" {
		t.Errorf("delete dest credentials = %+v, want resolved secrets", del.calls[0].dest)
	}
	if len(hs.deleted) != 1 || hs.deleted[0] != "bkh_1" {
		t.Errorf("deleted rows = %v, want [bkh_1]", hs.deleted)
	}
}

func TestDeleteRunner_DeleteBackup_BackupNotFound(t *testing.T) {
	hs := &fakeDeleteHistoryStore{fakeDownloadHistoryStore: fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{},
	}}
	d := &DeleteRunner{Store: hs, Secrets: newTestSecrets(), Deleter: &fakeDeleter{}}

	if err := d.DeleteBackup(context.Background(), "bkh_missing"); err == nil {
		t.Fatal("DeleteBackup() error = nil, want an error for a missing backup")
	}
	if len(hs.deleted) != 0 {
		t.Errorf("deleted rows = %v, want none for a lookup failure", hs.deleted)
	}
}

// TestDeleteRunner_DeleteBackup_ObjectAlreadyGone verifies the exact
// scenario this feature's own scope requires: a Deleter that reports the
// object is already gone (S3Deleter's own contract, backup.go's Deleter
// doc comment: a DeleteObject against a missing key is success, not an
// error) still lets the history row get deleted, not just "gracefully",
// but as the ordinary happy path.
func TestDeleteRunner_DeleteBackup_ObjectAlreadyGone(t *testing.T) {
	hs := &fakeDeleteHistoryStore{fakeDownloadHistoryStore: fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}}
	del := &fakeDeleter{} // no err: mirrors S3Deleter succeeding on an already-missing key
	d := &DeleteRunner{Store: hs, Secrets: newTestSecrets(), Deleter: del}

	if err := d.DeleteBackup(context.Background(), "bkh_1"); err != nil {
		t.Fatalf("DeleteBackup() error = %v", err)
	}
	if len(hs.deleted) != 1 {
		t.Fatalf("deleted rows = %v, want [bkh_1]", hs.deleted)
	}
}

// TestDeleteRunner_DeleteBackup_ObjectDeleteFails_StillDeletesRow is the
// core "never fail the whole request over a storage-side problem" contract
// DeleteRunner's own doc comment describes: the row is deleted anyway.
func TestDeleteRunner_DeleteBackup_ObjectDeleteFails_StillDeletesRow(t *testing.T) {
	hs := &fakeDeleteHistoryStore{fakeDownloadHistoryStore: fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}}
	del := &fakeDeleter{err: errors.New("s3: access denied")}
	d := &DeleteRunner{Store: hs, Secrets: newTestSecrets(), Deleter: del}

	if err := d.DeleteBackup(context.Background(), "bkh_1"); err != nil {
		t.Fatalf("DeleteBackup() error = %v, want nil despite the object delete failing", err)
	}
	if len(hs.deleted) != 1 {
		t.Fatalf("deleted rows = %v, want [bkh_1] even though the object delete failed", hs.deleted)
	}
}

// TestDeleteRunner_DeleteBackup_TargetNotFound_StillDeletesRow covers a
// deleted or otherwise unresolvable backup target: the object is
// unreachable from this control plane's own perspective either way, so
// this must not block cleaning up the row.
func TestDeleteRunner_DeleteBackup_TargetNotFound_StillDeletesRow(t *testing.T) {
	hs := &fakeDeleteHistoryStore{fakeDownloadHistoryStore: fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{},
	}}
	del := &fakeDeleter{}
	d := &DeleteRunner{Store: hs, Secrets: newTestSecrets(), Deleter: del}

	if err := d.DeleteBackup(context.Background(), "bkh_1"); err != nil {
		t.Fatalf("DeleteBackup() error = %v, want nil despite the target not resolving", err)
	}
	if len(hs.deleted) != 1 {
		t.Fatalf("deleted rows = %v, want [bkh_1]", hs.deleted)
	}
	if len(del.calls) != 0 {
		t.Errorf("Deleter.Delete calls = %d, want 0 since the destination never resolved", len(del.calls))
	}
}

// TestDeleteRunner_DeleteBackup_SecretResolveFails_StillDeletesRow mirrors
// TargetNotFound above for the other resolution failure mode.
func TestDeleteRunner_DeleteBackup_SecretResolveFails_StillDeletesRow(t *testing.T) {
	hs := &fakeDeleteHistoryStore{fakeDownloadHistoryStore: fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}}
	del := &fakeDeleter{}
	d := &DeleteRunner{Store: hs, Secrets: &fakeSecrets{err: errors.New("secrets: master key not configured")}, Deleter: del}

	if err := d.DeleteBackup(context.Background(), "bkh_1"); err != nil {
		t.Fatalf("DeleteBackup() error = %v, want nil despite secret resolution failing", err)
	}
	if len(hs.deleted) != 1 {
		t.Fatalf("deleted rows = %v, want [bkh_1]", hs.deleted)
	}
	if len(del.calls) != 0 {
		t.Errorf("Deleter.Delete calls = %d, want 0 since credentials never resolved", len(del.calls))
	}
}

// TestDeleteRunner_DeleteBackup_NilDeleter_OnlyDeletesRow covers
// DeleteRunner.Deleter's own documented nil-is-valid contract.
func TestDeleteRunner_DeleteBackup_NilDeleter_OnlyDeletesRow(t *testing.T) {
	hs := &fakeDeleteHistoryStore{fakeDownloadHistoryStore: fakeDownloadHistoryStore{
		backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
		targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
	}}
	d := &DeleteRunner{Store: hs, Secrets: newTestSecrets()}

	if err := d.DeleteBackup(context.Background(), "bkh_1"); err != nil {
		t.Fatalf("DeleteBackup() error = %v", err)
	}
	if len(hs.deleted) != 1 {
		t.Fatalf("deleted rows = %v, want [bkh_1]", hs.deleted)
	}
}

func TestDeleteRunner_DeleteBackup_StoreDeleteFails_PropagatesError(t *testing.T) {
	hs := &fakeDeleteHistoryStore{
		fakeDownloadHistoryStore: fakeDownloadHistoryStore{
			backups: map[string]store.BackupHistory{"bkh_1": newTestBackup()},
			targets: map[string]store.BackupTarget{"bkt_test": newTestTarget()},
		},
		deleteErr: errors.New("store: disk full"),
	}
	d := &DeleteRunner{Store: hs, Secrets: newTestSecrets(), Deleter: &fakeDeleter{}}

	if err := d.DeleteBackup(context.Background(), "bkh_1"); err == nil {
		t.Fatal("DeleteBackup() error = nil, want the store delete failure surfaced")
	}
}
