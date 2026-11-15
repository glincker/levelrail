//go:build !windows

package sourcecache

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestSparseCheckoutFetchesOnlyThePath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not found")
	}
	src := t.TempDir()
	for _, a := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@t"}, {"config", "user.name", "t"}, {"config", "uploadpack.allowFilter", "true"}, {"config", "uploadpack.allowAnySHA1InWant", "true"}} {
		if out, err := cleanGit(src, a...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", a, err, out)
		}
	}
	for p, body := range map[string]string{"stack/app.yaml": "kind: App\n", "big/blob.bin": string(make([]byte, 200<<10))} {
		full := filepath.Join(src, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, a := range [][]string{{"add", "."}, {"commit", "-q", "-m", "one"}} {
		if out, err := cleanGit(src, a...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", a, err, out)
		}
	}
	old := sparseProtocols
	sparseProtocols = "file"
	t.Cleanup(func() { sparseProtocols = old })

	c := New(Config{Dir: t.TempDir()}, nil)
	co, err := c.SparseCheckout(context.Background(), "file://"+src, "main", "stack", "", 1<<20, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer co.Cleanup()
	if _, err := os.Stat(filepath.Join(co.Dir, "stack", "app.yaml")); err != nil {
		t.Fatalf("wanted path missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(co.Dir, "big", "blob.bin")); err == nil {
		t.Fatal("path outside the sparse set was checked out")
	}
	if co.Commit == "" {
		t.Fatal("no commit")
	}
	if _, err := c.SparseCheckout(context.Background(), "file://"+src, "main", "stack", "", 10, 50*time.Millisecond); !errors.Is(err, ErrOverBudget) {
		t.Fatalf("want ErrOverBudget, got %v", err)
	}
}
