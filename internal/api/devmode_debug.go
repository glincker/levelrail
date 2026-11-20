//go:build !embedweb

package api

import "os"

// devModeEnabled reads APP_DEV_MODE directly (the one deliberate
// exception to NewRouter's "no environment reads inside internal/api"
// convention: this exists specifically to gate the dev-mode bypass
// itself, see devmode.go). Only present in non -tags embedweb builds;
// see devmode_release.go for the always-false counterpart a real
// release build compiles instead.
func devModeEnabled() bool {
	return os.Getenv("APP_DEV_MODE") == "1"
}

// devLoginConfig reads the dev-login token and public host. Present only in
// non-embedweb builds so a release binary can never enable the route.
func devLoginConfig() (token, publicHost string) {
	return os.Getenv("APP_DEV_LOGIN_TOKEN"), os.Getenv("APP_PUBLIC_HOST")
}
