// Package version holds the control plane's own build version, injected
// via -ldflags at release build time (see .github/workflows/release.yml).
package version

// Version is the running build's version string, e.g. "v1.2.3" for a
// tagged release. Defaults to "dev" for local and CI builds that don't
// pass -ldflags.
var Version = "dev"
