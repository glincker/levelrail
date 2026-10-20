//go:build !embedweb

package api

import "testing"

func TestDevModeEnabled_ReadsEnvVar(t *testing.T) {
	t.Setenv("APP_DEV_MODE", "1")
	if !devModeEnabled() {
		t.Error("devModeEnabled() = false, want true with APP_DEV_MODE=1 (non-embedweb build)")
	}
}

func TestDevModeEnabled_DefaultsFalse(t *testing.T) {
	t.Setenv("APP_DEV_MODE", "")
	if devModeEnabled() {
		t.Error("devModeEnabled() = true, want false with APP_DEV_MODE unset")
	}
}

func TestDevModeEnabled_RejectsNonExactValue(t *testing.T) {
	t.Setenv("APP_DEV_MODE", "true")
	if devModeEnabled() {
		t.Error("devModeEnabled() = true, want false: only the exact string \"1\" enables dev mode")
	}
}
