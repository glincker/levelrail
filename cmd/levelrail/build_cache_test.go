package main

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TestBuildCacheOptions exercises the env-var-to-build.Option mapping
// as pure logic: no docker daemon, no BuildKit connection, just
// which options get returned for a given set of env vars. The actual
// options' effect is internal/build's own responsibility, already
// covered there (cache_test.go, solve_test.go); this only checks
// buildCacheOptions wires the right env var to the right option.
func TestBuildCacheOptions(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		wantLen  int
		wantNone bool
	}{
		{
			name:     "no env vars set: no options",
			env:      map[string]string{},
			wantNone: true,
		},
		{
			name:    "cache dir only",
			env:     map[string]string{"APP_BUILD_CACHE_DIR": "/var/lib/levelrail-data/build-cache"},
			wantLen: 1,
		},
		{
			name:    "cache registry only",
			env:     map[string]string{"APP_BUILD_CACHE_REGISTRY": "registry.example.com/build-cache:app"},
			wantLen: 1,
		},
		{
			name: "cache registry with insecure",
			env: map[string]string{
				"APP_BUILD_CACHE_REGISTRY":          "registry.example.com/build-cache:app",
				"APP_BUILD_CACHE_REGISTRY_INSECURE": "true",
			},
			wantLen: 2,
		},
		{
			name: "insecure flag ignored without a registry ref set",
			env: map[string]string{
				"APP_BUILD_CACHE_REGISTRY_INSECURE": "true",
			},
			wantNone: true,
		},
		{
			name: "dir and registry both set",
			env: map[string]string{
				"APP_BUILD_CACHE_DIR":      "/var/lib/levelrail-data/build-cache",
				"APP_BUILD_CACHE_REGISTRY": "registry.example.com/build-cache:app",
			},
			wantLen: 2,
		},
	}

	db := openCredentialsTestDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"APP_BUILD_CACHE_DIR", "APP_BUILD_CACHE_REGISTRY", "APP_BUILD_CACHE_REGISTRY_INSECURE"} {
				t.Setenv(key, tt.env[key])
			}

			got := buildCacheOptions(context.Background(), db, logger)
			if tt.wantNone {
				if len(got) != 0 {
					t.Errorf("buildCacheOptions() = %d options, want 0", len(got))
				}
				return
			}
			if len(got) != tt.wantLen {
				t.Errorf("buildCacheOptions() = %d options, want %d", len(got), tt.wantLen)
			}
		})
	}
}

// TestBuildCacheOptions_BuiltinRegistry_AutoWired proves an enabled
// built-in registry with a Host set gets wired into the build cache
// automatically when no explicit APP_BUILD_CACHE_REGISTRY override is
// set, and that an explicit override always wins over it.
func TestBuildCacheOptions_BuiltinRegistry_AutoWired(t *testing.T) {
	for _, key := range []string{"APP_BUILD_CACHE_DIR", "APP_BUILD_CACHE_REGISTRY", "APP_BUILD_CACHE_REGISTRY_INSECURE"} {
		t.Setenv(key, "")
	}
	db := openCredentialsTestDB(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	ctx := context.Background()

	if got := buildCacheOptions(ctx, db, logger); len(got) != 0 {
		t.Fatalf("buildCacheOptions() = %d options, want 0 before the registry is enabled", len(got))
	}

	if err := db.UpdateRegistrySettings(ctx, store.RegistrySettings{Enabled: true, Host: "registry.internal.example", Username: "levelrail"}); err != nil {
		t.Fatalf("UpdateRegistrySettings() error = %v", err)
	}
	got := buildCacheOptions(ctx, db, logger)
	if len(got) != 2 {
		t.Fatalf("buildCacheOptions() = %d options, want 2 (registry ref + insecure) once the built-in registry is enabled", len(got))
	}

	t.Setenv("APP_BUILD_CACHE_REGISTRY", "external.example.com/cache:app")
	got = buildCacheOptions(ctx, db, logger)
	if len(got) != 1 {
		t.Errorf("buildCacheOptions() = %d options, want 1: an explicit APP_BUILD_CACHE_REGISTRY must win over the built-in registry", len(got))
	}
}
