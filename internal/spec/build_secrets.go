package spec

import (
	"fmt"
	"os"
	"strconv"
)

// EnvBuildSecretsMax overrides how many build.secrets one service may list.
const EnvBuildSecretsMax = "APP_BUILD_SECRETS_MAX"

const defaultBuildSecretsMax = 20

func buildSecretsMax() int {
	if v, err := strconv.Atoi(os.Getenv(EnvBuildSecretsMax)); err == nil && v > 0 {
		return v
	}
	return defaultBuildSecretsMax
}

// validateBuildSecrets checks build.secrets names, uniqueness, count and
// that the build type can mount BuildKit secrets.
func (b Build) validateBuildSecrets(name string) error {
	if len(b.Secrets) == 0 {
		return nil
	}
	if b.Type != BuildDockerfile {
		return fmt.Errorf("spec: service %q: build.secrets is not meaningful for build.type %q, only dockerfile builds can mount build secrets", name, b.Type)
	}
	if limit := buildSecretsMax(); len(b.Secrets) > limit {
		return fmt.Errorf("spec: service %q: build.secrets lists %d names, over the cap of %d (%s)", name, len(b.Secrets), limit, EnvBuildSecretsMax)
	}
	seen := make(map[string]bool, len(b.Secrets))
	for _, s := range b.Secrets {
		if !buildArgNameRe.MatchString(s) {
			return fmt.Errorf("spec: service %q: build.secrets name %q is not a valid variable name", name, s)
		}
		if seen[s] {
			return fmt.Errorf("spec: service %q: build.secrets lists %q more than once", name, s)
		}
		seen[s] = true
	}
	return nil
}
