package api

import (
	"errors"
	"fmt"
	"slices"
)

var (
	errAbilityRequired = errors.New("at least one ability is required")
	errRootExclusive   = errors.New("root cannot be combined with other abilities")
)

// unknownAbilityError names the specific bad ability string, so the
// resulting 400 response tells the caller exactly what to fix rather
// than a generic "invalid abilities."
type unknownAbilityError struct{ ability string }

func (e *unknownAbilityError) Error() string {
	return fmt.Sprintf("unknown ability %q", e.ability)
}

// Ability strings an api_tokens row can be scoped to (TASKS.md
// "Backend auth foundation"), adopted from Coolify's own model per
// docs-local/research/competitor-onboarding-auth-ux.md finding 9: a
// small, legible permission surface an MCP-issued token can be provably
// scoped to at the token layer itself, not just by convention in what a
// caller chooses to call. AbilityRoot is exclusive of the rest (checked
// at mint time, see validateAbilities), not additive with them.
const (
	AbilityRead           = "read"
	AbilityReadSensitive  = "read:sensitive"
	AbilityWrite          = "write"
	AbilityWriteSensitive = "write:sensitive"
	AbilityDeploy         = "deploy"
	AbilityRoot           = "root"
)

// validAbilities is every ability a token may be minted with, used to
// reject an unrecognized string at creation time rather than silently
// storing (and never matching) a typo.
var validAbilities = []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive, AbilityDeploy, AbilityRoot}

// hasAbility reports whether abilities grants required: either directly,
// or via AbilityRoot, which implies everything. A session (the single
// human admin, matching this phase's single-admin-user model of no teams
// and no RBAC yet) is never checked against this function at all, it is
// always treated as implicitly root, since there is exactly one identity
// to be; hasAbility only gates bearer-token callers, where scoping is
// the entire point.
func hasAbility(abilities []string, required string) bool {
	return slices.Contains(abilities, required) || slices.Contains(abilities, AbilityRoot)
}

// validateAbilities rejects an empty list, an unrecognized ability
// string, and AbilityRoot combined with anything else (Coolify's own
// rule: selecting root clears every other ability, root is exclusive,
// not additive).
func validateAbilities(abilities []string) error {
	if len(abilities) == 0 {
		return errAbilityRequired
	}
	if slices.Contains(abilities, AbilityRoot) && len(abilities) > 1 {
		return errRootExclusive
	}
	for _, a := range abilities {
		if !slices.Contains(validAbilities, a) {
			return &unknownAbilityError{ability: a}
		}
	}
	return nil
}
