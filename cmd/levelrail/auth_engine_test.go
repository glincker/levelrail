package main

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/brand"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestAuthEngineOptionsOffByDefault(t *testing.T) {
	t.Setenv("APP_AUTH_ENGINE", "")
	if got := authEngineOptions(context.Background(), slog.Default(), &brand.Brand{ShortName: "x"}, nil, nil); got != nil {
		t.Fatalf("flag off must return no options, got %d", len(got))
	}
	t.Setenv("APP_AUTH_ENGINE", "legacy")
	if got := authEngineOptions(context.Background(), slog.Default(), &brand.Brand{ShortName: "x"}, nil, nil); got != nil {
		t.Fatalf("legacy must return no options, got %d", len(got))
	}
}

func TestBackfillAuthDryRunPrintsCountsOnly(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, "levelrail.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	h := "$2a$10$abcdefghijklmnopqrstuuabcdefghijklmnopqrstuvwxyz01234"
	if err := db.CreateUser(ctx, store.User{ID: "user_x", Email: "a@b.test", DisplayName: "A", PasswordHash: &h, Abilities: []string{"root"}}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := backfillAuth(ctx, db, dir, true, &out); err != nil {
		t.Fatalf("backfillAuth: %v", err)
	}
	s := out.String()
	if !strings.Contains(s, "dry run") || !strings.Contains(s, "users copied:            1") {
		t.Fatalf("unexpected output: %s", s)
	}
	if strings.Contains(s, h) {
		t.Fatal("output leaked a password hash")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM theauth_users`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("dry run wrote rows: %d %v", n, err)
	}
}
