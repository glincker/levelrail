package spool

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSpoolSweepRemovesOnlyOldFiles(t *testing.T) {
	t.Setenv(envDir, t.TempDir())
	old, _ := Create("a")
	_ = old.Close()
	fresh, _ := Create("b")
	_ = fresh.Close()
	past := time.Now().Add(-2 * MaxAge())
	if err := os.Chtimes(old.Name(), past, past); err != nil {
		t.Fatal(err)
	}
	if got := Usage().Orphans; got != 1 {
		t.Fatalf("orphans = %d", got)
	}
	if n := Sweep(); n != 1 {
		t.Fatalf("swept %d", n)
	}
	if _, err := os.Stat(filepath.Join(Dir(), filepath.Base(fresh.Name()))); err != nil {
		t.Fatal("fresh file removed")
	}
}

func TestSpoolQuotaRefusesCreate(t *testing.T) {
	t.Setenv(envDir, t.TempDir())
	t.Setenv(envQuota, "10")
	f, err := Create("a")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("0123456789ab")
	_ = f.Close()
	if _, err := Create("b"); !errors.Is(err, ErrQuota) {
		t.Fatalf("err = %v", err)
	}
}
