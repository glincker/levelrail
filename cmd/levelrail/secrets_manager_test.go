package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

func TestLoadOrGenerateMasterKey_GeneratesOnFirstCall(t *testing.T) {
	dataDir := t.TempDir()
	mk, err := loadOrGenerateMasterKey(dataDir)
	if err != nil {
		t.Fatalf("loadOrGenerateMasterKey() error = %v", err)
	}
	if mk.String() == "" {
		t.Fatal("generated master key serializes to an empty string")
	}
}

func TestLoadOrGenerateMasterKey_PersistsAndReuses(t *testing.T) {
	dataDir := t.TempDir()
	first, err := loadOrGenerateMasterKey(dataDir)
	if err != nil {
		t.Fatalf("first loadOrGenerateMasterKey() error = %v", err)
	}

	second, err := loadOrGenerateMasterKey(dataDir)
	if err != nil {
		t.Fatalf("second loadOrGenerateMasterKey() error = %v", err)
	}

	if first.String() != second.String() {
		t.Error("second call generated a different key instead of reusing the persisted one: this would orphan every DEK wrapped under the first key")
	}
}

func TestLoadOrGenerateMasterKey_PersistedFilePath(t *testing.T) {
	dataDir := t.TempDir()
	if _, err := loadOrGenerateMasterKey(dataDir); err != nil {
		t.Fatalf("loadOrGenerateMasterKey() error = %v", err)
	}

	keyPath := filepath.Join(dataDir, masterKeyFilename)
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat persisted key file: %v", err)
	}
	if info.Size() == 0 {
		t.Error("persisted key file is empty")
	}
}

func TestLoadSecretsManager_EnvVarTakesPriorityOverPersistedFile(t *testing.T) {
	dataDir := t.TempDir()
	// Write garbage to the persisted key file's path: if loadSecretsManager
	// used it instead of APP_MASTER_KEY, LoadMasterKey would fail to parse
	// it and this test would catch that as an unexpected error.
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		t.Fatalf("mkdir data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, masterKeyFilename), []byte("not a valid master key"), 0o600); err != nil {
		t.Fatalf("seed invalid persisted key: %v", err)
	}

	envMK, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatalf("generate env master key: %v", err)
	}
	t.Setenv("APP_MASTER_KEY", envMK.String())

	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("closing test db: %v", err)
		}
	})

	mgr, err := loadSecretsManager(db, dataDir)
	if err != nil {
		t.Fatalf("loadSecretsManager() error = %v, want success: an env var should never even look at the invalid persisted file", err)
	}
	if mgr == nil {
		t.Fatal("loadSecretsManager() returned a nil manager")
	}
}
