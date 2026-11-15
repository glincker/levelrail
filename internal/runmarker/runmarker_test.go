package runmarker

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func vmstatFile(t *testing.T, n string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "vmstat")
	if err := os.WriteFile(p, []byte("nr_free_pages 10\noom_kill "+n+"\npgfault 5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBeginDetectsUncleanStopAndOOM(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	if _, inc := begin(dir, vmstatFile(t, "2"), now); inc != nil {
		t.Fatal("first boot must not report an incident")
	}
	_, inc := begin(dir, vmstatFile(t, "3"), now.Add(time.Minute))
	if inc == nil || !inc.OOMLikely {
		t.Fatalf("want unclean with oom, got %+v", inc)
	}
	if got := Recent(dir, time.Hour, now.Add(2*time.Minute)); len(got) != 1 {
		t.Fatalf("history: %+v", got)
	}
}

func TestCleanStopLeavesNoIncident(t *testing.T) {
	dir := t.TempDir()
	tr, _ := begin(dir, vmstatFile(t, "0"), time.Now())
	tr.Clean()
	if _, inc := begin(dir, vmstatFile(t, "0"), time.Now()); inc != nil {
		t.Fatalf("clean stop reported %+v", inc)
	}
}

func TestUncleanWithoutCounterIncreaseIsNotOOM(t *testing.T) {
	dir := t.TempDir()
	begin(dir, vmstatFile(t, "4"), time.Now())
	_, inc := begin(dir, vmstatFile(t, "4"), time.Now())
	if inc == nil || inc.OOMLikely {
		t.Fatalf("want unclean without oom, got %+v", inc)
	}
}

func TestMissingVmstatDegrades(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope")
	begin(dir, missing, time.Now())
	_, inc := begin(dir, missing, time.Now())
	if inc == nil || inc.OOMLikely {
		t.Fatalf("want unclean, no oom claim, got %+v", inc)
	}
}

func TestRecentWindowAndEnv(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	appendIncident(dir, Incident{At: now.Add(-100 * time.Hour)})
	if got := Recent(dir, 72*time.Hour, now); len(got) != 0 {
		t.Fatalf("old incident leaked: %+v", got)
	}
	if WindowFromEnv(func(string) string { return "1h" }) != time.Hour {
		t.Fatal("env window ignored")
	}
	if WindowFromEnv(func(string) string { return "" }) != defaultWindow {
		t.Fatal("default window")
	}
}
