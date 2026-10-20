//go:build embedweb

package api

// Release builds ignore APP_DEV_MODE entirely, so the bypass needs both a non-embedweb build and the env var.
func devModeEnabled() bool {
	return false
}
