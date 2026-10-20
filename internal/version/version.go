// Package version holds the control plane's own build version, injected
// via -ldflags at release build time (see .github/workflows/release.yml).
package version

import "runtime/debug"

// Version is the running build's version string, e.g. "v1.2.3" for a
// tagged release. Defaults to "dev" for local and CI builds that don't
// pass -ldflags; a binary installed with `go install module@vX.Y.Z`
// takes its version from the embedded module info instead.
var Version = "dev"

func init() {
	if info, ok := debug.ReadBuildInfo(); ok {
		Version = resolve(Version, info.Main.Version)
	}
}

// resolve keeps an ldflags-injected version and only falls back to the
// module version when none was injected and the module version is a real
// one (go build in a checkout reports "(devel)").
func resolve(injected, module string) string {
	if injected != "dev" || module == "" || module == "(devel)" {
		return injected
	}
	return module
}
