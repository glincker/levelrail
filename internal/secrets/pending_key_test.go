package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func passthrough(s string) (string, error) { return s, nil }

func seededManager(t *testing.T) (*Manager, *fakeStore, *MasterKey) {
	t.Helper()
	oldKey, err := GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	fs := newFakeStore()
	m := NewManager(fs, oldKey)
	if err := m.SetValue(context.Background(), "web", "API_KEY", "sk-abc"); err != nil { //nolint:gosec // fixture
		t.Fatal(err)
	}
	return m, fs, oldKey
}

func TestRecoverPendingMasterKey_CrashAfterDBCommitPromotesNewKey(t *testing.T) {
	ctx := context.Background()
	_, fs, oldKey := seededManager(t)
	path := filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(path, []byte(oldKey.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	newKey, _ := GenerateMasterKey()
	if err := StagePendingMasterKeyFile(path, newKey.String()); err != nil {
		t.Fatal(err)
	}
	// Simulated crash: DB rotation committed, file rename never ran.
	if err := RotateStoredDEKs(ctx, fs, oldKey, newKey); err != nil {
		t.Fatal(err)
	}

	restarted := NewManager(fs, oldKey) // restart loads the stale file key
	promoted, err := restarted.RecoverPendingMasterKey(ctx, path, passthrough)
	if err != nil || !promoted {
		t.Fatalf("RecoverPendingMasterKey = (%v, %v), want promoted", promoted, err)
	}
	if got, err := restarted.Resolve(ctx, "web", "API_KEY"); err != nil || got != "sk-abc" {
		t.Fatalf("Resolve after recovery = (%q, %v)", got, err)
	}
	if data, _ := os.ReadFile(path); string(data) != newKey.String() { //nolint:gosec // test path
		t.Error("key file was not replaced by the pending key")
	}
	if _, err := os.Stat(PendingMasterKeyPath(path)); !os.IsNotExist(err) {
		t.Error(".new still present after recovery")
	}
}

func TestRecoverPendingMasterKey_CrashBeforeDBCommitDiscardsStagedKey(t *testing.T) {
	ctx := context.Background()
	m, _, oldKey := seededManager(t)
	path := filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(path, []byte(oldKey.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	newKey, _ := GenerateMasterKey()
	if err := StagePendingMasterKeyFile(path, newKey.String()); err != nil {
		t.Fatal(err)
	}

	promoted, err := m.RecoverPendingMasterKey(ctx, path, passthrough)
	if err != nil || promoted {
		t.Fatalf("RecoverPendingMasterKey = (%v, %v), want not promoted", promoted, err)
	}
	if data, _ := os.ReadFile(path); string(data) != oldKey.String() { //nolint:gosec // test path
		t.Error("original key file must stay untouched")
	}
	if _, err := os.Stat(PendingMasterKeyPath(path)); !os.IsNotExist(err) {
		t.Error("stale .new should be discarded")
	}
	if got, err := m.Resolve(ctx, "web", "API_KEY"); err != nil || got != "sk-abc" {
		t.Fatalf("Resolve = (%q, %v)", got, err)
	}
}

func TestRecoverPendingMasterKey_UnrelatedPendingKeyIsRejectedAndKept(t *testing.T) {
	ctx := context.Background()
	_, fs, oldKey := seededManager(t)
	third, _ := GenerateMasterKey()
	other, _ := GenerateMasterKey()
	if err := RotateStoredDEKs(ctx, fs, oldKey, third); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "master.key")
	if err := os.WriteFile(path, []byte(oldKey.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := StagePendingMasterKeyFile(path, other.String()); err != nil {
		t.Fatal(err)
	}
	m := NewManager(fs, oldKey)
	if promoted, err := m.RecoverPendingMasterKey(ctx, path, passthrough); err == nil || promoted {
		t.Fatalf("RecoverPendingMasterKey = (%v, %v), want error", promoted, err)
	}
	if _, err := os.Stat(PendingMasterKeyPath(path)); err != nil {
		t.Error("pending key must be kept for manual recovery when nothing matches")
	}
}

func TestRecoverPendingMasterKey_NoPendingFileIsNoop(t *testing.T) {
	m, _, _ := seededManager(t)
	promoted, err := m.RecoverPendingMasterKey(context.Background(), filepath.Join(t.TempDir(), "master.key"), passthrough)
	if err != nil || promoted {
		t.Fatalf("= (%v, %v), want no-op", promoted, err)
	}
}
