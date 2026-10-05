package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAuthEngineOptionsBuildsTheEngine(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	opts, err := authEngineOptions(ctx, slog.Default(), &brand.Brand{ShortName: "x", Name: "x"}, db.DB, nil)
	if err != nil || len(opts) != 1 {
		t.Fatalf("authEngineOptions = %d options, err %v; want 1 option", len(opts), err)
	}
}
