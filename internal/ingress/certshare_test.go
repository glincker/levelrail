package ingress

import (
	"bytes"
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Two ingress processes on one database: what one stores the other loads, and
// they never hold the same issuance lock at once.
func twoIngressOnOneDB(t *testing.T) (*SQLiteStorage, *SQLiteStorage) {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewSQLiteStorage(db, testLogger(t)), NewSQLiteStorage(db, testLogger(t))
}

func TestSharedStorage_SecondIngressSeesFirstIngressCertificate(t *testing.T) {
	a, b := twoIngressOnOneDB(t)
	ctx := context.Background()
	key := "certificates/acme-v02/example.com/example.com.key"
	want := []byte("private-key-bytes")
	if err := a.Store(ctx, key, want); err != nil {
		t.Fatal(err)
	}
	got, err := b.Load(ctx, key)
	if err != nil {
		t.Fatalf("second ingress could not load what the first stored: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("second ingress loaded %q, want %q", got, want)
	}
	if !b.Exists(ctx, key) {
		t.Fatal("second ingress does not see the key")
	}
}

func TestSharedStorage_IssuanceLockExcludesTheOtherIngress(t *testing.T) {
	a, b := twoIngressOnOneDB(t)
	if err := a.Lock(context.Background(), "issue_cert_example.com"); err != nil {
		t.Fatal(err)
	}
	short, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := b.Lock(short, "issue_cert_example.com"); err == nil {
		t.Fatal("second ingress took a lock the first still holds: both would request the same certificate")
	}
	if err := a.Unlock(context.Background(), "issue_cert_example.com"); err != nil {
		t.Fatal(err)
	}
	if err := b.Lock(context.Background(), "issue_cert_example.com"); err != nil {
		t.Fatalf("lock not released for the second ingress: %v", err)
	}
	_ = b.Unlock(context.Background(), "issue_cert_example.com")
}

func TestSharedStorage_RacingIngressesNeverOverlap(t *testing.T) {
	a, b := twoIngressOnOneDB(t)
	var inside, overlaps atomic.Int32
	var wg sync.WaitGroup
	for _, s := range []*SQLiteStorage{a, b} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 3; i++ {
				if err := s.Lock(context.Background(), "issue_cert_race.example.com"); err != nil {
					t.Error(err)
					return
				}
				if inside.Add(1) > 1 {
					overlaps.Add(1)
				}
				time.Sleep(20 * time.Millisecond)
				inside.Add(-1)
				if err := s.Unlock(context.Background(), "issue_cert_race.example.com"); err != nil {
					t.Error(err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if overlaps.Load() != 0 {
		t.Fatalf("%d overlapping holders of one issuance lock", overlaps.Load())
	}
}
