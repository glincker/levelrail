//go:build !windows

package diskfree

import "testing"

func TestStatTempDir(t *testing.T) {
	info, err := Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if info.Total <= 0 || info.Free < 0 || info.Free > info.Total {
		t.Fatalf("unexpected usage %+v", info)
	}
}

func TestStatMissingPath(t *testing.T) {
	if _, err := Stat("/definitely/not/here"); err == nil {
		t.Fatal("expected an error")
	}
}
