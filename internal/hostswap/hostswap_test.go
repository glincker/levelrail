package hostswap

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func fakeProc(t *testing.T, swapKB int, swaps string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "proc", "sys", "vm"), 0o750); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"proc/meminfo":           "MemTotal: 1048576 kB\nSwapTotal: " + strconv.Itoa(swapKB) + " kB\n",
		"proc/swaps":             swaps,
		"proc/sys/vm/swappiness": "60\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestValidatePathAndSize(t *testing.T) {
	for _, ok := range []string{"/swapfile-levelrail", "/swapfile", "/var/lib/swap1"} {
		if err := ValidatePath(ok); err != nil {
			t.Errorf("ValidatePath(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "swapfile", "/etc/passwd", "/var/../etc/x", "/var/a b", "/var/a\nb", "/var/a;b", "/var//x", "/swapfile'x", "/var/x#y"} {
		if err := ValidatePath(bad); err == nil {
			t.Errorf("ValidatePath(%q) accepted", bad)
		}
	}
	st := State{Supported: true}
	if _, err := BuildPlan(st, "/swapfile-levelrail", MaxSizeMB+1, 1<<50, "/nonexistent-fstab"); err == nil {
		t.Error("oversized swapfile accepted")
	}
	if err := Apply(context.Background(), Plan{Path: "/etc/passwd", SizeMB: 10}, "/nonexistent-fstab"); err == nil {
		t.Error("Apply accepted a path outside the allowed directories")
	}
}

func TestHostSwapPlan(t *testing.T) {
	st := Read(fakeProc(t, 0, "Filename Type Size Used Priority\n"))
	if !st.Supported || st.RAMBytes != 1<<30 || st.SwapBytes != 0 || st.Swappiness != 60 {
		t.Fatalf("state = %+v", st)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sw")
	allowedPrefixes = append(allowedPrefixes, dir+"/")
	t.Cleanup(func() { allowedPrefixes = allowedPrefixes[:2] })
	fstab := filepath.Join(dir, "fstab")
	size := SizeMB(0, st.RAMBytes)
	if size != 1024 {
		t.Fatalf("auto size = %d", size)
	}
	p, err := BuildPlan(st, path, size, 3<<30, fstab)
	if err != nil || len(p.Commands) != 5 || len(p.Notes) != 1 {
		t.Fatalf("plan = %+v err = %v", p, err)
	}
	if _, err := BuildPlan(st, path, size, 1<<30, fstab); err == nil {
		t.Fatal("expected a disk space refusal")
	}
	if err := os.WriteFile(fstab, []byte("/swap.img none swap sw 0 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BuildPlan(st, path, size, 3<<30, fstab); err == nil {
		t.Fatal("expected a refusal with an existing fstab swap entry")
	}
	active := Read(fakeProc(t, 524288, "Filename Type Size Used Priority\n/swap.img file 524284 0 -2\n"))
	if _, err := BuildPlan(active, path, size, 3<<30, filepath.Join(dir, "none")); err == nil || len(active.Swapfiles) != 1 {
		t.Fatalf("expected a refusal with active swap, err = %v", err)
	}
}
