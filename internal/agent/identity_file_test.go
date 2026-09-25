package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testIdentity(tag string) *Identity {
	return &Identity{NodeID: "node-1", ClientCertPEM: []byte("cert-" + tag), ClientKeyPEM: []byte("key-" + tag), CACertPEM: []byte("ca")}
}

func TestIdentityFile_SaveLoadIsPrivate(t *testing.T) {
	f := NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
	if err := f.Save(testIdentity("a")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := f.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if string(got.ClientKeyPEM) != "key-a" || got.NodeID != "node-1" {
		t.Fatalf("Load() = %+v", got)
	}
	info, err := os.Stat(f.Path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("identity file mode = %o, want 600", perm)
	}
}

func TestIdentityFile_LoadMissing(t *testing.T) {
	f := NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
	if _, err := f.Load(); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Load() error = %v, want os.ErrNotExist", err)
	}
}

func TestIdentityFile_CrashBetweenWriteAndRename(t *testing.T) {
	dir := t.TempDir()
	f := NewIdentityFile(filepath.Join(dir, "identity.json"))
	if err := f.Save(testIdentity("old")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// The process dies after fsyncing the temp file and before rename: the
	// temp file is left behind and the target still holds the old identity.
	crashed := errors.New("simulated crash before rename")
	f.rename = func(string, string) error { return crashed }
	if err := f.Save(testIdentity("new")); !errors.Is(err, crashed) {
		t.Fatalf("Save() error = %v, want the simulated crash", err)
	}
	stale := filepath.Join(dir, "identity.json"+identityTempInfix+"left-by-crash")
	if err := os.WriteFile(stale, []byte("partial"), 0o600); err != nil {
		t.Fatalf("write stale temp: %v", err)
	}

	restarted := NewIdentityFile(f.Path)
	got, err := restarted.Load()
	if err != nil {
		t.Fatalf("Load() after crash error = %v", err)
	}
	if string(got.ClientCertPEM) != "cert-old" {
		t.Fatalf("Load() after crash = %q, want the old identity intact", got.ClientCertPEM)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale temp file survived Load: %v", err)
	}
}

func TestIdentityFile_StageConfirmRollback(t *testing.T) {
	f := NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
	if err := f.Save(testIdentity("old")); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if err := f.Stage(testIdentity("new")); err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	if !f.HasStaged() {
		t.Fatal("HasStaged() = false after Stage")
	}
	if prev, _ := f.LoadPrev(); string(prev.ClientCertPEM) != "cert-old" {
		t.Fatalf("prev = %q, want old", prev.ClientCertPEM)
	}
	if err := f.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	if got, _ := f.Load(); string(got.ClientCertPEM) != "cert-old" || f.HasStaged() {
		t.Fatalf("after rollback: %q staged=%v", got.ClientCertPEM, f.HasStaged())
	}

	if err := f.Stage(testIdentity("new")); err != nil {
		t.Fatalf("Stage() error = %v", err)
	}
	if err := f.Confirm(); err != nil {
		t.Fatalf("Confirm() error = %v", err)
	}
	if got, _ := f.Load(); string(got.ClientCertPEM) != "cert-new" || f.HasStaged() {
		t.Fatalf("after confirm: %q staged=%v", got.ClientCertPEM, f.HasStaged())
	}
}

func TestIdentityFile_ResolveStaged(t *testing.T) {
	unreachable := status.Error(codes.Unavailable, "connection refused")
	rejected := status.Error(codes.Unauthenticated, "fingerprint mismatch")
	tests := []struct {
		name       string
		checkErr   error
		wantCert   string
		wantStaged bool
	}{
		{"new cert accepted is confirmed", nil, "cert-new", false},
		{"new cert rejected rolls back", rejected, "cert-old", false},
		{"control plane unreachable keeps both", unreachable, "cert-new", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
			_ = f.Save(testIdentity("old"))
			if err := f.Stage(testIdentity("new")); err != nil {
				t.Fatalf("Stage() error = %v", err)
			}
			got, err := f.ResolveStaged(context.Background(), func(context.Context, *Identity) error { return tt.checkErr })
			if err != nil {
				t.Fatalf("ResolveStaged() error = %v", err)
			}
			if string(got.ClientCertPEM) != tt.wantCert || f.HasStaged() != tt.wantStaged {
				t.Fatalf("got %q staged=%v, want %q staged=%v", got.ClientCertPEM, f.HasStaged(), tt.wantCert, tt.wantStaged)
			}
		})
	}
}
