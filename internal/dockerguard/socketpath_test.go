package dockerguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultSocketPathDarwinShort(t *testing.T) {
	long := "/Users/someone/Library/Application Support/" + strings.Repeat("deep/", 20) + "levelrail-agent"
	got := defaultSocketPath(long, "darwin", 501)
	if len(got) >= maxSocketPathLen {
		t.Fatalf("path %q is %d bytes", got, len(got))
	}
	if linux := defaultSocketPath(long, "linux", 501); linux != filepath.Join(long, "guard", SocketName) {
		t.Fatalf("linux path changed: %s", linux)
	}
	dir := filepath.Join(t.TempDir(), "lr-test")
	if err := privateDir(dir); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode %v err %v", info, err)
	}
}
