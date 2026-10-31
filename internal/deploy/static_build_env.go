package deploy

import (
	"os"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// defaultStaticBuildEnvPrefixes are the conventions frameworks use for values
// that are meant to end up in the published bundle.
const defaultStaticBuildEnvPrefixes = "VITE_,NEXT_PUBLIC_,PUBLIC_,GATSBY_"

var staticBuildEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// StaticBuildEnvPrefixes reads APP_STATIC_BUILD_ENV_PREFIXES, the name prefixes
// of app env vars that reach a static build. Empty after trimming turns the
// feature off.
func StaticBuildEnvPrefixes() []string {
	raw, ok := os.LookupEnv("APP_STATIC_BUILD_ENV_PREFIXES")
	if !ok {
		raw = defaultStaticBuildEnvPrefixes
	}
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// staticBuildArgs adds the app's public env to the build args. Only literal
// values under an allowed prefix are passed: secrets, references and vault
// values never enter a build that publishes its output. Explicit build.args win.
func staticBuildArgs(env map[string]spec.EnvVar, args map[string]string, prefixes []string) map[string]string {
	out := make(map[string]string, len(args)+len(env))
	for k, v := range env {
		if v.Value == "" || v.Secret || v.From != "" || v.Vault != nil || !staticBuildEnvName.MatchString(k) {
			continue
		}
		for _, p := range prefixes {
			if strings.HasPrefix(k, p) {
				out[k] = v.Value
				break
			}
		}
	}
	for k, v := range args {
		out[k] = v
	}
	return out
}
