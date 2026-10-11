package dockerguard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListenRejectsUnsafeSocketPaths(t *testing.T) {
	base := shortTempDir(t)
	openDir := filepath.Join(base, "open")
	if err := os.Mkdir(openDir, 0o755); err != nil { //nolint:gosec // deliberately too open
		t.Fatal(err)
	}
	if err := os.Chmod(openDir, 0o755); err != nil { //nolint:gosec // deliberately too open
		t.Fatal(err)
	}
	notSocket := filepath.Join(base, "priv", "file.sock")
	if err := os.MkdirAll(filepath.Dir(notSocket), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(notSocket, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, socket, wantErr string
	}{
		{name: "too long", socket: filepath.Join(base, strings.Repeat("a", 120), SocketName), wantErr: EnvSocket},
		{name: "group readable dir", socket: filepath.Join(openDir, SocketName), wantErr: "want 0700"},
		{name: "existing non socket", socket: notSocket, wantErr: "not a socket"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := New(Config{Mode: ModeEnforce, Upstream: "/nonexistent.sock", Tunables: Tunables{RecordQueue: 1}})
			_, err := Listen(context.Background(), g, tt.socket)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Listen(%s) err = %v, want %q", tt.socket, err, tt.wantErr)
			}
		})
	}
}
