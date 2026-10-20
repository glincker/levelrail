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

// Ability strings an api_tokens row can be scoped to. AbilityRoot is exclusive: validateAbilities rejects it combined with others.
const (
	AbilityRead           = "read"
	AbilityReadSensitive  = "read:sensitive"
	AbilityWrite          = "write"
	AbilityWriteSensitive = "write:sensitive"
	AbilityDeploy         = "deploy"
	AbilityRoot           = "root"
	// AbilitySignInApprove lets a token reveal its owner's sign-in codes and
	// decide new-browser approvals. Root never implies it.
	AbilitySignInApprove = "signin:approve"
)

// validAbilities is every ability a token may be minted with, used to
// reject an unrecognized string at creation time rather than silently
// storing (and never matching) a typo.
var validAbilities = []string{AbilityRead, AbilityReadSensitive, AbilityWrite, AbilityWriteSensitive, AbilityDeploy, AbilityRoot}

// validTokenAbilities adds the token-only abilities a user or role never holds.
var validTokenAbilities = append(slices.Clone(validAbilities), AbilitySignInApprove, AbilitySecretsReveal)

// hasSignInApprove reports whether abilities literally carry
// AbilitySignInApprove, never by way of root or write:sensitive.
func hasSignInApprove(abilities []string) bool {
	return slices.Contains(abilities, AbilitySignInApprove)
}

// hasAbility grants required directly or via AbilityRoot, which implies everything.
func hasAbility(abilities []string, required string) bool {
	return slices.Contains(abilities, required) || slices.Contains(abilities, AbilityRoot)
}

// validateAbilities rejects an empty list, unknown abilities, and AbilityRoot combined with anything else.
func validateAbilities(abilities []string) error {
	return validateAbilitiesIn(abilities, validAbilities)
}

// validateTokenAbilities is validateAbilities for an API token mint.
func validateTokenAbilities(abilities []string) error {
	return validateAbilitiesIn(abilities, validTokenAbilities)
}

func validateAbilitiesIn(abilities, valid []string) error {
	if len(abilities) == 0 {
		return errAbilityRequired
	}
	if slices.Contains(abilities, AbilityRoot) && len(abilities) > 1 {
		return errRootExclusive
	}
	for _, a := range abilities {
		if !slices.Contains(valid, a) {
			return &unknownAbilityError{ability: a}
		}
	}
	return nil
}
