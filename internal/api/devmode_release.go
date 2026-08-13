//go:build embedweb

package api

// devModeEnabled always returns false in a real -tags embedweb build,
// without even reading APP_DEV_MODE: mirrors web/embed_real.go's
// polarity so that shipping the dev-mode bypass requires both omitting
// -tags embedweb (which already breaks the embedded frontend, a loud
// failure) and setting APP_DEV_MODE=1. See devmode_debug.go for the
// real, env-reading counterpart a non-embedweb build compiles instead.
func devModeEnabled() bool {
	return false
}
