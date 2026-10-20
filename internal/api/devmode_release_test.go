//go:build embedweb

package api

import "testing"

func TestDevModeEnabled_AlwaysFalse_Embedweb(t *testing.T) {
	t.Setenv("APP_DEV_MODE", "1")
	if devModeEnabled() {
		t.Error("devModeEnabled() = true, want false: an -tags embedweb build must never honor APP_DEV_MODE")
	}
}
