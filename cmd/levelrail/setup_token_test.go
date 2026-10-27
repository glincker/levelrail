package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/api"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestRunSetupToken(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("APP_DATA_DIR", dataDir)
	t.Setenv("APP_PUBLIC_HOST", "203.0.113.7")
	t.Setenv("APP_HTTP_ADDR", ":9090")
	openFresh := func(context.Context) (*store.DB, error) {
		return store.Open(context.Background(), filepath.Join(dataDir, "levelrail.db"))
	}

	if err := runSetupToken(context.Background(), &bytes.Buffer{}, false, openFresh); err == nil {
		t.Fatal("runSetupToken() with no database = nil, want an error pointing at APP_DATA_DIR")
	}
	initDB, err := openFresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = initDB.Close()

	var first bytes.Buffer
	if err := runSetupToken(context.Background(), &first, false, openFresh); err != nil {
		t.Fatalf("runSetupToken() error = %v", err)
	}
	token, err := api.ReadSetupToken(dataDir)
	if err != nil || token == "" {
		t.Fatalf("ReadSetupToken() = %q, %v", token, err)
	}
	want := "http://203.0.113.7:9090/login#setup=" + token
	if !strings.Contains(first.String(), token) || !strings.Contains(first.String(), want) {
		t.Errorf("output = %q, want token and %s", first.String(), want)
	}

	var second bytes.Buffer
	if err := runSetupToken(context.Background(), &second, false, openFresh); err != nil {
		t.Fatal(err)
	}
	if second.String() != first.String() {
		t.Errorf("second run printed %q, want the same token as %q", second.String(), first.String())
	}

	adminDB, err := openFresh(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := api.BootstrapAdmin(context.Background(), adminDB, "admin", "a-real-password"); err != nil {
		t.Fatal(err)
	}
	_ = adminDB.Close()
	if err := runSetupToken(context.Background(), &bytes.Buffer{}, false, openFresh); !errors.Is(err, errAdminExists) {
		t.Errorf("runSetupToken() with an admin = %v, want errAdminExists", err)
	}
	if _, err := os.Stat(api.SetupTokenPath(dataDir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("setup token file should be gone once an admin exists: %v", err)
	}
}

func TestEnsureSetupTokenNeverLogsToken(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("APP_DATA_DIR", dataDir)
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(dataDir, "levelrail.db"))
	if err != nil {
		t.Fatalf("store.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	// First call creates the token, second reuses it: neither may log it.
	ensureSetupToken(ctx, logger, db)
	ensureSetupToken(ctx, logger, db)

	token, err := api.ReadSetupToken(dataDir)
	if err != nil || token == "" {
		t.Fatalf("ReadSetupToken() = %q, %v", token, err)
	}
	if strings.Contains(logs.String(), token) {
		t.Fatalf("setup token leaked into log output: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "setup-token") {
		t.Fatalf("log output does not point at the setup-token subcommand: %s", logs.String())
	}
}

func TestRunSetupTokenPlain(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("APP_DATA_DIR", dataDir)
	open := func(context.Context) (*store.DB, error) {
		return store.Open(context.Background(), filepath.Join(dataDir, "levelrail.db"))
	}
	db, err := open(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	var out bytes.Buffer
	if err := runSetupToken(context.Background(), &out, true, open); err != nil {
		t.Fatal(err)
	}
	token, _ := api.ReadSetupToken(dataDir)
	if out.String() != token+"\n" {
		t.Errorf("plain output = %q, want only the token", out.String())
	}
}
