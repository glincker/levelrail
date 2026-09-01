package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Effect is a statement's outcome when it matches: Allow or Deny,
// exactly AWS IAM's own two-value vocabulary.
type Effect string

// EffectAllow and EffectDeny are the only two valid Statement.Effect
// values.
const (
	EffectAllow Effect = "Allow"
	EffectDeny  Effect = "Deny"
)

// Statement is one Allow/Deny rule inside a Document: Action holds
// ability strings (abilities.go's AbilityRead etc.) or "*", Resource
// holds resource identifiers ("app:myapp", "database:main") or "*".
// Both are lists so one statement can cover several actions or
// resources at once, matching how a real IAM statement reads.
type Statement struct {
	Effect   Effect   `json:"Effect"`
	Action   []string `json:"Action"`
	Resource []string `json:"Resource"`
}

// Document is the whole parsed policy body stored as JSON in
// store.Policy.Document.
type Document struct {
	Statement []Statement `json:"Statement"`
}

var (
	errDocumentNoStatements = errors.New("policy document must contain at least one statement")
	errStatementNoAction    = errors.New("statement must list at least one action")
	errStatementNoResource  = errors.New("statement must list at least one resource")
	errStatementBadEffect   = errors.New("statement Effect must be \"Allow\" or \"Deny\"")
)

// unknownActionError names the specific bad action string, mirroring
// unknownAbilityError's own reasoning: a 400 should say exactly what to
// fix, not just that something was wrong.
type unknownActionError struct{ action string }

func (e *unknownActionError) Error() string {
	return fmt.Sprintf("unknown action %q", e.action)
}

// ParseDocument unmarshals and validates a policy document. A document
// must have at least one statement, every statement must have an
// Effect of Allow or Deny, at least one Action (each either "*" or a
// known ability string from validAbilities), and at least one
// Resource (each either "*" or a non-empty resource identifier).
func ParseDocument(raw string) (*Document, error) {
	var doc Document
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, fmt.Errorf("parse policy document: %w", err)
	}
	if len(doc.Statement) == 0 {
		return nil, errDocumentNoStatements
	}
	for i := range doc.Statement {
		if err := validateStatement(doc.Statement[i]); err != nil {
			return nil, err
		}
	}
	return &doc, nil
}

// validateStatement checks one Statement in isolation: ParseDocument's
// own per-statement rules, split out to keep ParseDocument itself a
// flat, low-complexity loop.
func validateStatement(s Statement) error {
	if s.Effect != EffectAllow && s.Effect != EffectDeny {
		return errStatementBadEffect
	}
	if len(s.Action) == 0 {
		return errStatementNoAction
	}
	for _, a := range s.Action {
		if a != "*" && !isKnownAbility(a) {
			return &unknownActionError{action: a}
		}
	}
	if len(s.Resource) == 0 {
		return errStatementNoResource
	}
	for _, r := range s.Resource {
		if strings.TrimSpace(r) == "" {
			return errStatementNoResource
		}
	}
	return nil
}

func isKnownAbility(a string) bool {
	for _, v := range validAbilities {
		if v == a {
			return true
		}
	}
	return false
}

// matchesPattern reports whether value matches pattern, where pattern
// is either "*" (matches anything), an exact string, or a prefix
// ending in "*" (e.g. "app:*" matches "app:web", "app:api", ...): the
// same trailing-wildcard convention AWS ARNs use in a Resource entry.
func matchesPattern(pattern, value string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}

func statementMatches(s Statement, ability, resource string) bool {
	actionMatch := false
	for _, a := range s.Action {
		if matchesPattern(a, ability) {
			actionMatch = true
			break
		}
	}
	if !actionMatch {
		return false
	}
	for _, r := range s.Resource {
		if matchesPattern(r, resource) {
			return true
		}
	}
	return false
}

// evaluatePolicies applies AWS IAM's own evaluation order across every
// attached policy's statements: an explicit Deny anywhere always wins,
// regardless of how many Allow statements also match. It returns
// (matched, allowed): matched is false when no statement in any policy
// mentions this (ability, resource) pair at all, letting the caller
// fall back to the base ability check unchanged.
func evaluatePolicies(policies []store.Policy, ability, resource string) (matched, allowed bool) {
	sawAllow := false
	for _, p := range policies {
		doc, err := ParseDocument(p.Document)
		if err != nil {
			// A malformed stored document can't grant or deny anything;
			// treat it as if it simply doesn't mention this pair.
			continue
		}
		for _, s := range doc.Statement {
			if !statementMatches(s, ability, resource) {
				continue
			}
			if s.Effect == EffectDeny {
				return true, false
			}
			sawAllow = true
		}
	}
	return sawAllow, sawAllow
}

// authorizeResource is the resource-scoped counterpart to hasAbility:
// call it wherever a route needs to check permission on one specific
// resource rather than only the route-level ability. Evaluation order,
// matching real IAM: (1) an explicit Deny in any attached policy always
// wins; (2) otherwise the caller's existing flat baseAbilities decide,
// exactly today's requireAbility behavior, so a principal with no
// attached policies is completely unaffected; (3) otherwise an explicit
// Allow in an attached policy can still grant an ability scoped to this
// resource that baseAbilities does not grant globally.
func authorizeResource(baseAbilities []string, policies []store.Policy, ability, resource string) bool {
	matched, allowed := evaluatePolicies(policies, ability, resource)
	if matched && !allowed {
		return false
	}
	if hasAbility(baseAbilities, ability) {
		return true
	}
	return matched && allowed
}
