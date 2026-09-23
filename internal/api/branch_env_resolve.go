package api

import (
	"context"
	"fmt"
	"path"
	"sort"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// branchMatchesPattern reports whether branch matches pattern, a shell
// glob per path.Match: "*" matches any run of non-"/" characters, "?"
// matches one, "[...]" a character class. "release/*" therefore
// matches "release/foo" but not "release/foo/bar" or "main"; a
// wildcard-free pattern only matches an identical branch name.
func branchMatchesPattern(branch, pattern string) (bool, error) {
	matched, err := path.Match(pattern, branch)
	if err != nil {
		return false, fmt.Errorf("branch pattern %q: %w", pattern, err)
	}
	return matched, nil
}

// applyBranchEnvOverrides is applyPreviewEnvOverrides' branch-scoped
// counterpart: for every override in overrides whose BranchPattern
// matches branch, replaces svcSpec.Env[key] with a literal value,
// winning over both svcSpec's own inherited Env/SecretEnv/VaultEnv and
// any unscoped preview-env override applyPreviewEnvOverrides already
// applied to svcSpec -- the resolution order this feature exists for is
// branch-specific override > app-level env > nothing. A secret-marked
// override's plaintext is decrypted fresh here via secretResolver,
// never cached, the same discipline sharedenv.Resolver already holds
// for shared env var secrets.
//
// When two overrides target the same key and both match branch, an
// exact (non-glob) pattern wins over a glob pattern, since it names
// this branch specifically rather than a family of branches; two
// matching glob patterns for the same key resolve deterministically
// (the lexicographically last pattern string wins) but that ordering
// isn't otherwise meaningful, an intentionally rare case an operator
// should avoid by not declaring overlapping patterns for one key.
func applyBranchEnvOverrides(ctx context.Context, svcSpec *spec.Service, serviceName, branch string, overrides []store.ServiceBranchEnvOverride, secretResolver SecretSetter) error {
	if branch == "" || len(overrides) == 0 {
		return nil
	}

	type match struct {
		override store.ServiceBranchEnvOverride
		exact    bool
	}
	matches := make(map[string]match, len(overrides))
	for _, o := range overrides {
		ok, err := branchMatchesPattern(branch, o.BranchPattern)
		if err != nil || !ok {
			continue
		}
		exact := o.BranchPattern == branch
		existing, have := matches[o.Key]
		switch {
		case !have:
			matches[o.Key] = match{override: o, exact: exact}
		case existing.exact && !exact:
			// keep the existing exact match, an override glob never wins.
		case !existing.exact && exact:
			matches[o.Key] = match{override: o, exact: exact}
		case o.BranchPattern > existing.override.BranchPattern:
			matches[o.Key] = match{override: o, exact: exact}
		}
	}
	if len(matches) == 0 {
		return nil
	}

	keys := make([]string, 0, len(matches))
	for k := range matches {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	if svcSpec.Env == nil {
		svcSpec.Env = make(map[string]spec.EnvVar, len(matches))
	}
	for _, k := range keys {
		m := matches[k]
		value := m.override.Value
		if m.override.Secret {
			if secretResolver == nil {
				return fmt.Errorf("branch env override %q for %q requires a secret resolver but none is configured", k, serviceName)
			}
			namespace := store.BranchEnvOverrideSecretsKey(serviceName, m.override.BranchPattern)
			resolved, err := secretResolver.Resolve(ctx, namespace, k)
			if err != nil {
				return fmt.Errorf("resolve branch env override %q for %q: %w", k, serviceName, err)
			}
			value = resolved
		}
		svcSpec.Env[k] = spec.EnvVar{Value: value}
	}
	return nil
}
