// Package storetest holds shared store helpers for tests.
package storetest

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Open returns a fresh store in a temp dir, closed when the test ends.
func Open(t testing.TB) *store.DB {
	t.Helper()
	db, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "x.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
