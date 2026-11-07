package deploy

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// defaultStaticBuildEnvPrefixes are the conventions frameworks use for values
// that are meant to end up in the published bundle.
const defaultStaticBuildEnvPrefixes = "VITE_,NEXT_PUBLIC_,PUBLIC_,GATSBY_,REACT_APP_,NUXT_PUBLIC_,EXPO_PUBLIC_"

var staticBuildEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// StaticBuildEnvPrefixes reads APP_STATIC_BUILD_ENV_PREFIXES, the name prefixes
// of app env vars that reach every framework build (static, Railpack, Dockerfile). Empty after trimming turns the
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

// StaticBuildArgsForTest exposes the build-arg filter so other packages can prove secrets never reach a build.
func StaticBuildArgsForTest(env map[string]spec.EnvVar) map[string]string {
	return staticBuildArgs(env, nil, StaticBuildEnvPrefixes())
}

// Setting keys for Next.js skew protection, registered in internal/api.
const (
	KeyNextDeploymentID    = "deploy.next_deployment_id"
	KeyNextAssetGraceSecs  = "deploy.next_asset_grace_seconds"
	EnvNextDeploymentID    = "APP_DEPLOY_NEXT_DEPLOYMENT_ID"
	EnvNextAssetGraceSecs  = "APP_DEPLOY_NEXT_ASSET_GRACE_SECONDS"
	nextDeploymentIDEnvVar = "NEXT_DEPLOYMENT_ID"
)

// nextSkewSettings holds the live setting readers, wired once at boot.
var nextSkewSettings atomic.Pointer[NextSkewSettings]

// NextSkewSettings reads the runtime settings this package needs.
type NextSkewSettings struct {
	DeploymentIDOn func(context.Context) bool
	GraceSeconds   func(context.Context) int
}

// SetNextSkewSettings wires the saved settings in at boot; nil restores the env seeds.
func SetNextSkewSettings(s *NextSkewSettings) { nextSkewSettings.Store(s) }

func nextDeploymentIDEnabled(ctx context.Context) bool {
	if s := nextSkewSettings.Load(); s != nil && s.DeploymentIDOn != nil {
		return s.DeploymentIDOn(ctx)
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvNextDeploymentID))) {
	case "0", "false", "off", "no":
		return false
	}
	return true
}

// nextAssetGrace returns the grace window, 0 when disabled.
func nextAssetGrace(ctx context.Context) time.Duration {
	n := 1800
	if s := nextSkewSettings.Load(); s != nil && s.GraceSeconds != nil {
		n = s.GraceSeconds(ctx)
	} else if raw, ok := os.LookupEnv(EnvNextAssetGraceSecs); ok {
		if v, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
			n = v
		}
	}
	if n <= 0 {
		return 0
	}
	return time.Duration(n) * time.Second
}

// nextDeploymentID returns the id to inject, or "" when the app is not Next.js,
// the setting is off, or the user already set NEXT_DEPLOYMENT_ID themselves.
func nextDeploymentID(ctx context.Context, sourceDir, deployID string, env map[string]spec.EnvVar) string {
	if deployID == "" || !nextDeploymentIDEnabled(ctx) {
		return ""
	}
	if _, set := env[nextDeploymentIDEnvVar]; set {
		return ""
	}
	if !build.DetectNext(sourceDir).Detected {
		return ""
	}
	return deployID
}
