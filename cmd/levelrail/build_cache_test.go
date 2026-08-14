package main

import "testing"

// TestBuildCacheOptions exercises TASKS.md 3.5's env-var-to-build.Option
// mapping as pure logic: no docker daemon, no BuildKit connection, just
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, key := range []string{"APP_BUILD_CACHE_DIR", "APP_BUILD_CACHE_REGISTRY", "APP_BUILD_CACHE_REGISTRY_INSECURE"} {
				t.Setenv(key, tt.env[key])
			}

			got := buildCacheOptions()
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
