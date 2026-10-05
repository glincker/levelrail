package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIdentityFile_CheckWritable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	tests := []struct {
		name    string
		setup   func(t *testing.T) string
		wantErr string
	}{
		{"existing writable dir", func(t *testing.T) string { return t.TempDir() }, ""},
		{"missing dir is created", func(t *testing.T) string { return filepath.Join(t.TempDir(), "a", "b") }, ""},
		{"read-only dir", func(t *testing.T) string {
			d := t.TempDir()
			if err := os.Chmod(d, 0o500); err != nil { //nolint:gosec // read-only dir needs the x bit
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(d, 0o700) }) //nolint:gosec // restore so TempDir can clean up
			return d
		}, "join token was not used"},
		{"uncreatable dir", func(t *testing.T) string {
			d := t.TempDir()
			if err := os.Chmod(d, 0o500); err != nil { //nolint:gosec // read-only dir needs the x bit
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(d, 0o700) }) //nolint:gosec // restore so TempDir can clean up
			return filepath.Join(d, "sub")
		}, "join token was not used"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.setup(t)
			err := NewIdentityFile(filepath.Join(dir, "identity.json")).CheckWritable()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("CheckWritable() error = %v", err)
				}
				if m, _ := filepath.Glob(filepath.Join(dir, "*probe*")); len(m) != 0 {
					t.Fatalf("probe file left behind: %v", m)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("CheckWritable() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
