package store

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"testing"
	"time"
)

func TestCertStorage_SaveGetRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveCertStorageValue(ctx, "acme/example/accounts/a.json", []byte(`{"status":"valid"}`)); err != nil {
		t.Fatalf("SaveCertStorageValue() error = %v", err)
	}

	got, err := db.GetCertStorageValue(ctx, "acme/example/accounts/a.json")
	if err != nil {
		t.Fatalf("GetCertStorageValue() error = %v", err)
	}
	if string(got.Value) != `{"status":"valid"}` {
		t.Errorf("Value = %q, want %q", got.Value, `{"status":"valid"}`)
	}
	if got.ModifiedAt.IsZero() {
		t.Error("ModifiedAt is zero, want a real timestamp")
	}
}

func TestCertStorage_SaveOverwritesExisting(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveCertStorageValue(ctx, "k", []byte("v1")); err != nil {
		t.Fatalf("first SaveCertStorageValue() error = %v", err)
	}
	first, err := db.GetCertStorageValue(ctx, "k")
	if err != nil {
		t.Fatalf("first GetCertStorageValue() error = %v", err)
	}

	time.Sleep(2 * time.Millisecond) // ensure a distinguishable modified_at
	if err := db.SaveCertStorageValue(ctx, "k", []byte("v2")); err != nil {
		t.Fatalf("second SaveCertStorageValue() error = %v", err)
	}
	second, err := db.GetCertStorageValue(ctx, "k")
	if err != nil {
		t.Fatalf("second GetCertStorageValue() error = %v", err)
	}

	if string(second.Value) != "v2" {
		t.Errorf("Value after overwrite = %q, want v2", second.Value)
	}
	if !second.ModifiedAt.After(first.ModifiedAt) {
		t.Errorf("ModifiedAt after overwrite = %v, want after first save's %v", second.ModifiedAt, first.ModifiedAt)
	}
}

func TestCertStorage_GetMissing_ReturnsErrNotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.GetCertStorageValue(ctx, "does/not/exist"); !errors.Is(err, ErrCertStorageKeyNotFound) {
		t.Errorf("GetCertStorageValue() error = %v, want ErrCertStorageKeyNotFound", err)
	}
}

func TestCertStorage_Delete_RemovesExactKeyAndChildren(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	for _, k := range []string{
		"acme/example/sites/foo.com/foo.com.crt",
		"acme/example/sites/foo.com/foo.com.key",
		"acme/example/sites/bar.com/bar.com.crt",
	} {
		if err := db.SaveCertStorageValue(ctx, k, []byte("x")); err != nil {
			t.Fatalf("SaveCertStorageValue(%q) error = %v", k, err)
		}
	}

	if err := db.DeleteCertStorageValue(ctx, "acme/example/sites/foo.com"); err != nil {
		t.Fatalf("DeleteCertStorageValue() error = %v", err)
	}

	if exists, err := db.ExistsCertStorageValue(ctx, "acme/example/sites/foo.com/foo.com.crt"); err != nil || exists {
		t.Errorf("foo.com.crt exists = %v (err=%v), want false: deleting a directory prefix must remove its children", exists, err)
	}
	if exists, err := db.ExistsCertStorageValue(ctx, "acme/example/sites/bar.com/bar.com.crt"); err != nil || !exists {
		t.Errorf("bar.com.crt exists = %v (err=%v), want true: deleting one prefix must not touch an unrelated sibling", exists, err)
	}
}

func TestCertStorage_Delete_MissingKeyIsNotAnError(t *testing.T) {
	db := openTestDB(t)
	if err := db.DeleteCertStorageValue(context.Background(), "never/existed"); err != nil {
		t.Errorf("DeleteCertStorageValue() on a missing key error = %v, want nil (idempotent, matches certmagic.FileStorage.Delete)", err)
	}
}

func TestCertStorage_Exists(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveCertStorageValue(ctx, "acme/example/sites/foo.com/foo.com.crt", []byte("x")); err != nil {
		t.Fatalf("SaveCertStorageValue() error = %v", err)
	}

	tests := []struct {
		name string
		key  string
		want bool
	}{
		{"exact leaf key", "acme/example/sites/foo.com/foo.com.crt", true},
		{"directory prefix of a leaf key", "acme/example/sites/foo.com", true},
		{"unrelated key", "acme/example/sites/nope.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.ExistsCertStorageValue(ctx, tt.key)
			if err != nil {
				t.Fatalf("ExistsCertStorageValue(%q) error = %v", tt.key, err)
			}
			if got != tt.want {
				t.Errorf("ExistsCertStorageValue(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestCertStorage_ListCertStorageKeys(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	keys := []string{
		"acme/example/sites/foo.com/foo.com.crt",
		"acme/example/sites/foo.com/foo.com.key",
		"acme/example/sites/bar.com/bar.com.crt",
	}
	for _, k := range keys {
		if err := db.SaveCertStorageValue(ctx, k, []byte("x")); err != nil {
			t.Fatalf("SaveCertStorageValue(%q) error = %v", k, err)
		}
	}

	t.Run("non-recursive collapses to immediate children", func(t *testing.T) {
		got, err := db.ListCertStorageKeys(ctx, "acme/example/sites", false)
		if err != nil {
			t.Fatalf("ListCertStorageKeys() error = %v", err)
		}
		sort.Strings(got)
		want := []string{"acme/example/sites/bar.com", "acme/example/sites/foo.com"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ListCertStorageKeys(recursive=false) = %v, want %v", got, want)
		}
	})

	t.Run("recursive returns every leaf", func(t *testing.T) {
		got, err := db.ListCertStorageKeys(ctx, "acme/example/sites", true)
		if err != nil {
			t.Fatalf("ListCertStorageKeys() error = %v", err)
		}
		sort.Strings(got)
		want := []string{
			"acme/example/sites/bar.com/bar.com.crt",
			"acme/example/sites/foo.com/foo.com.crt",
			"acme/example/sites/foo.com/foo.com.key",
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ListCertStorageKeys(recursive=true) = %v, want %v", got, want)
		}
	})

	t.Run("no matches returns an empty slice, not an error", func(t *testing.T) {
		got, err := db.ListCertStorageKeys(ctx, "acme/example/sites/nope.com", true)
		if err != nil {
			t.Fatalf("ListCertStorageKeys() error = %v", err)
		}
		if len(got) != 0 {
			t.Errorf("ListCertStorageKeys() = %v, want empty", got)
		}
	})
}

func TestCertStorage_StatTerminalAndDirectory(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveCertStorageValue(ctx, "acme/example/sites/foo.com/foo.com.crt", []byte("hello")); err != nil {
		t.Fatalf("SaveCertStorageValue() error = %v", err)
	}

	leaf, err := db.StatCertStorageValue(ctx, "acme/example/sites/foo.com/foo.com.crt")
	if err != nil {
		t.Fatalf("StatCertStorageValue(leaf) error = %v", err)
	}
	if !leaf.IsTerminal || leaf.Size != 5 {
		t.Errorf("leaf stat = %+v, want IsTerminal=true Size=5", leaf)
	}

	dir, err := db.StatCertStorageValue(ctx, "acme/example/sites/foo.com")
	if err != nil {
		t.Fatalf("StatCertStorageValue(directory) error = %v", err)
	}
	if dir.IsTerminal {
		t.Errorf("directory stat = %+v, want IsTerminal=false", dir)
	}

	if _, err := db.StatCertStorageValue(ctx, "does/not/exist"); !errors.Is(err, ErrCertStorageKeyNotFound) {
		t.Errorf("StatCertStorageValue(missing) error = %v, want ErrCertStorageKeyNotFound", err)
	}
}

func TestCertStorage_AcquireLock_FreshLockBlocksCompetitor(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.AcquireCertStorageLock(ctx, "issue-foo.com", time.Minute)
	if err != nil {
		t.Fatalf("first AcquireCertStorageLock() error = %v", err)
	}
	if !got {
		t.Fatal("first AcquireCertStorageLock() = false, want true (lock was free)")
	}

	got, err = db.AcquireCertStorageLock(ctx, "issue-foo.com", time.Minute)
	if err != nil {
		t.Fatalf("second AcquireCertStorageLock() error = %v", err)
	}
	if got {
		t.Fatal("second AcquireCertStorageLock() = true, want false: a fresh lock must not be stealable")
	}
}

func TestCertStorage_AcquireLock_StaleLockIsStolen(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.AcquireCertStorageLock(ctx, "issue-foo.com", time.Minute); err != nil {
		t.Fatalf("first AcquireCertStorageLock() error = %v", err)
	}

	// A staleAfter of 0 means "even a lock acquired an instant ago counts
	// as stale," simulating a competitor showing up long after the
	// original holder crashed without releasing it.
	got, err := db.AcquireCertStorageLock(ctx, "issue-foo.com", 0)
	if err != nil {
		t.Fatalf("second AcquireCertStorageLock() error = %v", err)
	}
	if !got {
		t.Error("second AcquireCertStorageLock() = false, want true: a stale lock must be stealable")
	}
}

func TestCertStorage_ReleaseLock_FreesItForReacquisition(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.AcquireCertStorageLock(ctx, "issue-foo.com", time.Minute); err != nil {
		t.Fatalf("AcquireCertStorageLock() error = %v", err)
	}
	if err := db.ReleaseCertStorageLock(ctx, "issue-foo.com"); err != nil {
		t.Fatalf("ReleaseCertStorageLock() error = %v", err)
	}

	got, err := db.AcquireCertStorageLock(ctx, "issue-foo.com", time.Minute)
	if err != nil {
		t.Fatalf("re-acquire error = %v", err)
	}
	if !got {
		t.Error("re-acquire after release = false, want true")
	}
}

func TestCertStorage_ReleaseLock_MissingLockIsNotAnError(t *testing.T) {
	db := openTestDB(t)
	if err := db.ReleaseCertStorageLock(context.Background(), "never-acquired"); err != nil {
		t.Errorf("ReleaseCertStorageLock() on a never-held lock error = %v, want nil (idempotent)", err)
	}
}

func TestCertStorage_TouchLock_KeepsItFresh(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, err := db.AcquireCertStorageLock(ctx, "touched", time.Hour); err != nil {
		t.Fatalf("AcquireCertStorageLock(touched) error = %v", err)
	}
	if _, err := db.AcquireCertStorageLock(ctx, "untouched", time.Hour); err != nil {
		t.Fatalf("AcquireCertStorageLock(untouched) error = %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if err := db.TouchCertStorageLock(ctx, "touched"); err != nil {
		t.Fatalf("TouchCertStorageLock() error = %v", err)
	}

	// A 20ms staleness threshold: the untouched lock is now ~50ms old
	// and must be stealable, while the touched lock was just refreshed
	// and must not be, proving TouchCertStorageLock actually moved
	// updated_at forward rather than being a no-op.
	const staleAfter = 20 * time.Millisecond

	gotTouched, err := db.AcquireCertStorageLock(ctx, "touched", staleAfter)
	if err != nil {
		t.Fatalf("AcquireCertStorageLock(touched) error = %v", err)
	}
	if gotTouched {
		t.Error("touched lock was stolen despite being refreshed moments ago")
	}

	gotUntouched, err := db.AcquireCertStorageLock(ctx, "untouched", staleAfter)
	if err != nil {
		t.Fatalf("AcquireCertStorageLock(untouched) error = %v", err)
	}
	if !gotUntouched {
		t.Error("untouched lock was not stolen despite being older than staleAfter")
	}
}
