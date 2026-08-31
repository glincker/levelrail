package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeTargetTestStore struct {
	targets map[string]store.BackupTarget
	getErr  error
}

func (f *fakeTargetTestStore) GetBackupTarget(_ context.Context, id string) (store.BackupTarget, error) {
	if f.getErr != nil {
		return store.BackupTarget{}, f.getErr
	}
	target, ok := f.targets[id]
	if !ok {
		return store.BackupTarget{}, store.ErrBackupTargetNotFound
	}
	return target, nil
}

type fakeTester struct {
	gotDest Destination
	err     error
}

func (f *fakeTester) Test(_ context.Context, dest Destination) error {
	f.gotDest = dest
	return f.err
}

func TestTargetTester_TestTarget_Success(t *testing.T) {
	targetStore := &fakeTargetTestStore{targets: map[string]store.BackupTarget{
		"bkt_1": {ID: "bkt_1", Provider: "r2", Endpoint: "https://x.r2.cloudflarestorage.com", Region: "auto", Bucket: "backups"},
	}}
	secrets := &fakeSecrets{values: map[string]string{
		"backup-target/bkt_1/access_key_id":     "AKID",
		"backup-target/bkt_1/secret_access_key": "shh",
	}}
	tester := &fakeTester{}

	tt := &TargetTester{Store: targetStore, Secrets: secrets, Tester: tester}
	if err := tt.TestTarget(context.Background(), "bkt_1"); err != nil {
		t.Fatalf("TestTarget() error = %v", err)
	}

	want := Destination{Provider: "r2", Endpoint: "https://x.r2.cloudflarestorage.com", Region: "auto", Bucket: "backups", AccessKeyID: "AKID", SecretAccessKey: "shh"}
	if tester.gotDest != want {
		t.Errorf("Tester.Test called with %+v, want %+v", tester.gotDest, want)
	}
}

func TestTargetTester_TestTarget_TargetNotFound(t *testing.T) {
	tt := &TargetTester{Store: &fakeTargetTestStore{targets: map[string]store.BackupTarget{}}, Secrets: &fakeSecrets{}, Tester: &fakeTester{}}
	if err := tt.TestTarget(context.Background(), "bkt_missing"); err == nil {
		t.Fatal("TestTarget() error = nil, want the missing target surfaced")
	}
}

func TestTargetTester_TestTarget_SecretResolveFails(t *testing.T) {
	targetStore := &fakeTargetTestStore{targets: map[string]store.BackupTarget{
		"bkt_1": {ID: "bkt_1", Provider: "aws", Bucket: "backups"},
	}}
	secrets := &fakeSecrets{err: errors.New("master key not configured")}
	tester := &fakeTester{}

	tt := &TargetTester{Store: targetStore, Secrets: secrets, Tester: tester}
	if err := tt.TestTarget(context.Background(), "bkt_1"); err == nil {
		t.Fatal("TestTarget() error = nil, want the secret resolve failure surfaced")
	}
}

func TestTargetTester_TestTarget_TesterFails(t *testing.T) {
	targetStore := &fakeTargetTestStore{targets: map[string]store.BackupTarget{
		"bkt_1": {ID: "bkt_1", Provider: "aws", Bucket: "backups"},
	}}
	secrets := &fakeSecrets{values: map[string]string{
		"backup-target/bkt_1/access_key_id":     "AKID",
		"backup-target/bkt_1/secret_access_key": "shh",
	}}
	tester := &fakeTester{err: errors.New("403 Forbidden")}

	tt := &TargetTester{Store: targetStore, Secrets: secrets, Tester: tester}
	if err := tt.TestTarget(context.Background(), "bkt_1"); err == nil {
		t.Fatal("TestTarget() error = nil, want the tester's failure surfaced")
	}
}
