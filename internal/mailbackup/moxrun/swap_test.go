package moxrun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runSwap(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{"-c", swapScript, "swap", args[0], root}, args[1:]...)...) //nolint:gosec // test runs the script under test
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p) //nolint:gosec // test path
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func liveRoot(t *testing.T) string {
	r := t.TempDir()
	write(t, filepath.Join(r, "accounts", "a", "msg"), "old")
	write(t, filepath.Join(r, "index.db"), "oldidx")
	write(t, filepath.Join(r, "tmp", "x"), "t")
	return r
}

func stage(t *testing.T, r, idx string) {
	const id = "ID1"
	write(t, filepath.Join(r, WorkDir, "restore-"+id, "data", "accounts", "b", "msg"), "new")
	write(t, filepath.Join(r, WorkDir, "restore-"+id, "data", "index.db"), idx)
}

func mustSwap(t *testing.T, r string, args ...string) {
	t.Helper()
	if out, err := runSwap(t, r, args...); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, out)
	}
}

func TestSwapThenRollbackRestoresOriginal(t *testing.T) {
	r := liveRoot(t)
	stage(t, r, "newidx")
	mustSwap(t, r, "swap", "ID1", "pre-restore-1-ID1", "data")
	if got := read(t, filepath.Join(r, "index.db")); got != "newidx" {
		t.Fatalf("after swap index = %q", got)
	}
	if got := read(t, filepath.Join(r, "tmp", "x")); got != "t" {
		t.Fatalf("tmp must stay in place, got %q", got)
	}
	mustSwap(t, r, "rollback")
	if read(t, filepath.Join(r, "index.db")) != "oldidx" || read(t, filepath.Join(r, "accounts", "a", "msg")) != "old" {
		t.Fatal("rollback did not restore the original data")
	}
	if _, err := os.Stat(filepath.Join(r, "accounts", "b")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("restored entries must leave the live path on rollback")
	}
	if out, _ := runSwap(t, r, "state"); strings.TrimSpace(out) != "none" {
		t.Fatalf("journal left after rollback: %q", out)
	}
}

func TestInterruptedAsideRollsBack(t *testing.T) {
	r := liveRoot(t)
	stage(t, r, "newidx")
	pre := filepath.Join(r, WorkDir, "pre-restore-1-ID1")
	write(t, filepath.Join(r, WorkDir, "swap"), "ID1 pre-restore-1-ID1 aside\n")
	if err := os.MkdirAll(pre, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(r, "accounts"), filepath.Join(pre, "accounts")); err != nil {
		t.Fatal(err)
	}
	// A mox that came up on the half-empty dir and made a fresh file.
	write(t, filepath.Join(r, "accounts", "fresh"), "empty")
	mustSwap(t, r, "rollback")
	if read(t, filepath.Join(r, "accounts", "a", "msg")) != "old" || read(t, filepath.Join(r, "index.db")) != "oldidx" {
		t.Fatal("interrupted swap was not rolled back to the original")
	}
}

func TestSwapRefusesUnfinishedJournalAndCommitCapsCopies(t *testing.T) {
	r := liveRoot(t)
	write(t, filepath.Join(r, WorkDir, "swap"), "ID0 pre-restore-0-ID0 in\n")
	stage(t, r, "a")
	if _, err := runSwap(t, r, "swap", "ID1", "pre-restore-1-ID1", "data"); err == nil {
		t.Fatal("swap must refuse while an earlier swap is unfinished")
	}
	if err := os.Remove(filepath.Join(r, WorkDir, "swap")); err != nil {
		t.Fatal(err)
	}
	for i, pre := range []string{"pre-restore-1-ID1", "pre-restore-2-ID1", "pre-restore-3-ID1"} {
		if i > 0 {
			stage(t, r, pre)
		}
		mustSwap(t, r, "swap", "ID1", pre, "data")
		mustSwap(t, r, "commit", "", "", "2")
	}
	ents, _ := filepath.Glob(filepath.Join(r, WorkDir, "pre-restore-*"))
	if len(ents) != 2 || !strings.HasSuffix(ents[0], "pre-restore-2-ID1") {
		t.Fatalf("want the 2 newest pre-restore copies, got %v", ents)
	}
	if read(t, filepath.Join(r, "index.db")) != "pre-restore-3-ID1" {
		t.Fatal("the last restore is not live")
	}
}

type fakeExec struct{ r io.Reader }

func (f fakeExec) Exec(_ context.Context, _ string, _ []string) (io.ReadCloser, error) {
	return io.NopCloser(f.r), nil
}

type failAtEOF struct{ r io.Reader }

func (f failAtEOF) Read(p []byte) (int, error) {
	n, err := f.r.Read(p)
	if errors.Is(err, io.EOF) {
		return n, errors.New("exited 1")
	}
	return n, err
}

func TestExecRunnerSeesExitPastOutputCap(t *testing.T) {
	big := bytes.Repeat([]byte("x"), maxExecOutput+4096)
	_, err := ExecRunner{RT: fakeExec{failAtEOF{bytes.NewReader(big)}}}.Run(context.Background(), "backup")
	if err == nil {
		t.Fatal("a failing exec with more than the output cap must not read as success")
	}
	out, err := ExecRunner{RT: fakeExec{bytes.NewReader(big)}}.Run(context.Background(), "backup")
	if err != nil || len(out) != maxExecOutput {
		t.Fatalf("got len %d err %v", len(out), err)
	}
}

// TestOnlySwapCreatesTheWorkDir models the real-host failure: every mode but
// swap must leave a volume root without .levelrail when nothing is pending.
func TestOnlySwapCreatesTheWorkDir(t *testing.T) {
	r := liveRoot(t)
	work := filepath.Join(r, WorkDir)
	gone := func(when string) {
		t.Helper()
		if _, err := os.Stat(work); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: %s exists", when, work)
		}
	}
	for _, args := range [][]string{{"state"}, {"rollback"}, {"commit", "", "", "2"}, {"sweep", "", "", "120"}} {
		mustSwap(t, r, args...)
		gone(args[0] + " with nothing pending")
	}
	write(t, filepath.Join(work, "swap.new"), "ID1 pre-restore-1-ID1 aside\n")
	mustSwap(t, r, "sweep", "", "", "120")
	gone("sweep of an empty work dir with a closed journal")

	write(t, filepath.Join(work, "swap"), "ID1 pre-restore-1-ID1 in\n")
	if out, _ := runSwap(t, r, "sweep", "", "", "120"); strings.TrimSpace(out) != "busy" {
		t.Fatalf("sweep with an open journal said %q, want busy", out)
	}
	if err := os.Remove(filepath.Join(work, "swap")); err != nil {
		t.Fatal(err)
	}
	stage(t, r, "newidx")
	mustSwap(t, r, "swap", "ID1", "pre-restore-1-ID1", "data")
	if fi, err := os.Stat(work); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("swap work dir: %v %v", fi, err)
	}
	mustSwap(t, r, "commit", "", "", "0")
	gone("commit that keeps no pre-restore copy")
	stage(t, r, "again")
	mustSwap(t, r, "swap", "ID1", "pre-restore-2-ID1", "data")
	mustSwap(t, r, "rollback")
	if ents, _ := os.ReadDir(work); len(ents) != 1 || !strings.HasPrefix(ents[0].Name(), "discarded-") {
		t.Fatalf("rollback must leave only the discarded copy for the sweep, got %v", ents)
	}
}

func TestSweepReportsAnOpenJournal(t *testing.T) {
	err := Sweep(context.Background(), fakeExec{r: strings.NewReader("busy\n")}, "c", DataDir, "120")
	if !errors.Is(err, ErrSwapOpen) {
		t.Fatalf("an open journal must refuse the backup, got %v", err)
	}
	if err := Sweep(context.Background(), fakeExec{r: strings.NewReader("swept\n")}, "c", DataDir, "120"); err != nil {
		t.Fatal(err)
	}
}
