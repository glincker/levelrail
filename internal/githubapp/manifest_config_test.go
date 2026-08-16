package githubapp

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadManifestConfig_MissingFile_ReturnsDefaults(t *testing.T) {
	cfg, err := LoadManifestConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("LoadManifestConfig() error = %v, want nil for a missing file", err)
	}
	if !reflect.DeepEqual(cfg, DefaultManifestConfig()) {
		t.Errorf("LoadManifestConfig() = %+v, want DefaultManifestConfig()", cfg)
	}
}

func TestLoadManifestConfig_ParsesRealFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "github-app-manifest.yaml")
	writeFile(t, path, `
default_permissions:
  contents: read
  metadata: read
  issues: read
default_events:
  - push
`)

	cfg, err := LoadManifestConfig(path)
	if err != nil {
		t.Fatalf("LoadManifestConfig() error = %v", err)
	}
	if cfg.DefaultPermissions["issues"] != "read" {
		t.Errorf("DefaultPermissions[issues] = %q, want %q", cfg.DefaultPermissions["issues"], "read")
	}
	if len(cfg.DefaultEvents) != 1 || cfg.DefaultEvents[0] != "push" {
		t.Errorf("DefaultEvents = %v, want [push]", cfg.DefaultEvents)
	}
}

func TestLoadManifestConfig_InvalidYAML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	writeFile(t, path, "not: valid: yaml: at: all: [")

	if _, err := LoadManifestConfig(path); err == nil {
		t.Error("LoadManifestConfig() error = nil, want a parse error")
	}
}

func TestLoadManifestConfig_EmptyFile_NilFieldsBecomeEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.yaml")
	writeFile(t, path, "")

	cfg, err := LoadManifestConfig(path)
	if err != nil {
		t.Fatalf("LoadManifestConfig() error = %v", err)
	}
	if cfg.DefaultPermissions == nil {
		t.Error("DefaultPermissions = nil, want a non-nil (possibly empty) map")
	}
	if cfg.DefaultEvents == nil {
		t.Error("DefaultEvents = nil, want a non-nil (possibly empty) slice")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
