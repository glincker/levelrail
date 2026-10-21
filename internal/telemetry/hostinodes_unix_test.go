//go:build !windows

package telemetry

import "testing"

func TestHostInodes(t *testing.T) {
	total, free, err := HostInodes(t.TempDir())
	if err != nil {
		t.Fatalf("HostInodes on a temp dir: %v", err)
	}
	if free > total {
		t.Fatalf("free inodes %d exceed total %d", free, total)
	}
}

func TestHostInodesMissingPath(t *testing.T) {
	if _, _, err := HostInodes(t.TempDir() + "/does-not-exist"); err == nil {
		t.Fatal("expected an error for a missing path")
	}
}
