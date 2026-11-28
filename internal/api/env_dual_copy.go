package api

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/glincker/levelrail/internal/store"
)

// envSecretOverlap returns the sorted keys present both as plain env and as a declared secret.
func envSecretOverlap(env map[string]string, secretNames []string) []string {
	var out []string
	for _, k := range secretNames {
		if _, ok := env[k]; ok && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	slices.Sort(out)
	return out
}

// newEnvSecretOverlap lists overlaps a write would add. Overlaps the app
// already had are left alone so an old dual copy never blocks an unrelated save.
func newEnvSecretOverlap(env map[string]string, secretNames []string, oldEnv map[string]string, oldSecretNames []string) []string {
	had := envSecretOverlap(oldEnv, oldSecretNames)
	var out []string
	for _, k := range envSecretOverlap(env, secretNames) {
		if !slices.Contains(had, k) {
			out = append(out, k)
		}
	}
	return out
}

func dualCopyMessage(keys []string) string {
	return strings.Join(keys, ", ") + " would exist as both a plain variable and a secret; keep one: use apps secrets set to replace the plain copy, or apps secrets delete first"
}

// doctorCheckEnvDualCopy lists apps whose keys sit in both plain env and secrets. It is read only.
func (rt *Router) doctorCheckEnvDualCopy(ctx context.Context) doctorCheckResource {
	const code, name = "env_dual_copy", "Plain and secret variables"
	svcs, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not list apps: %s", err)}
	}
	var lines []string
	for _, s := range svcs {
		for _, k := range envSecretOverlap(s.Env, store.SecretEnvNames(s.SecretEnv)) {
			lines = append(lines, fmt.Sprintf("%s %s: run levelrail-cli apps env unset %s %s (the secret is kept)", s.Name, k, s.Name, k))
		}
	}
	if len(lines) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "no key is stored as both a plain variable and a secret"}
	}
	slices.Sort(lines)
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: strings.Join(lines, "; "), DocsPath: "/environment-and-secrets"}
}
