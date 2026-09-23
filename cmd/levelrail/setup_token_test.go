package main

import (
	"bytes"
	"context"
	"errors"
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

	var first bytes.Buffer
	if err := runSetupToken(context.Background(), &first, openFresh); err != nil {
		t.Fatalf("runSetupToken() error = %v", err)
	}
	token, err := api.ReadSetupToken(dataDir)
	if err != nil || token == "" {
		t.Fatalf("ReadSetupToken() = %q, %v", token, err)
	}
	want := "http://203.0.113.7:9090/login?setup=" + token
	if !strings.Contains(first.String(), token) || !strings.Contains(first.String(), want) {
		t.Errorf("output = %q, want token and %s", first.String(), want)
	}

	var second bytes.Buffer
	if err := runSetupToken(context.Background(), &second, openFresh); err != nil {
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
	if err := runSetupToken(context.Background(), &bytes.Buffer{}, openFresh); !errors.Is(err, errAdminExists) {
		t.Errorf("runSetupToken() with an admin = %v, want errAdminExists", err)
	}
	if _, err := os.Stat(api.SetupTokenPath(dataDir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("setup token file should be gone once an admin exists: %v", err)
	}
}
