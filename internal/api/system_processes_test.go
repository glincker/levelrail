package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadHostProcessesBoundedAndSorted(t *testing.T) {
	root := t.TempDir()
	for pid, rss := range map[string]string{"10": "100 kB", "11": "900 kB", "12": "500 kB"} {
		dir := filepath.Join(root, pid)
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		body := "Name:\tproc" + pid + "\nThreads:\t2\nVmRSS:\t" + rss + "\n"
		if err := os.WriteFile(filepath.Join(dir, "status"), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	got, total := readHostProcesses(root, 2)
	if total != 3 || len(got) != 2 || got[0].PID != 11 || got[0].RSSBytes != 900<<10 {
		t.Fatalf("got %+v total %d", got, total)
	}
}
