package cpbackup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestManager_Verify(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	m := NewManager(openDB(t), dir)
	info, err := m.Create(ctx)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	res, err := m.Verify(ctx, info.Name)
	if err != nil || !res.OK || len(res.Checks) != 3 {
		t.Fatalf("verify = %+v, %v; want ok with 3 checks", res, err)
	}
	list, err := m.List()
	if err != nil || len(list) != 1 || list[0].VerifiedOK == nil || !*list[0].VerifiedOK || list[0].VerifiedAt == nil {
		t.Fatalf("list after verify = %+v, %v", list, err)
	}

	path := filepath.Join(dir, DirName, info.Name)
	data, err := os.ReadFile(path) //nolint:gosec // test temp path
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	data[len(data)/2] ^= 0xff
	if err := os.WriteFile(path, data, 0o600); err != nil { //nolint:gosec // test temp path
		t.Fatalf("corrupt: %v", err)
	}
	res, err = m.Verify(ctx, info.Name)
	if err != nil || res.OK || res.Checks[0].OK {
		t.Fatalf("verify corrupted = %+v, %v; want not ok with checksum failing", res, err)
	}
	list, _ = m.List()
	if list[0].VerifiedOK == nil || *list[0].VerifiedOK {
		t.Fatalf("list should record the failed verification: %+v", list[0])
	}

	if _, err := m.Verify(ctx, "bad"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("bad name err = %v", err)
	}
	if _, err := m.Verify(ctx, "levelrail-20200101T000000Z.db"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing err = %v", err)
	}
	if err := m.Delete(info.Name); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if left, _ := os.ReadDir(filepath.Join(dir, DirName)); len(left) != 0 {
		t.Fatalf("sidecars left behind: %v", left)
	}
}
