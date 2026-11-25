// Package dbauthz is the single place that decides whether a caller may use a
// database explorer route (ADR 082). Every explorer route goes through Resolve.
package dbauthz

import (
	"context"
	"errors"
	"strings"
)

// Action is a database explorer action, also valid in IAM policy documents.
type Action string

// The db actions, named as in IAM policy documents.
const (
	ActionRead   Action = "db:read"
	ActionQuery  Action = "db:query"
	ActionWrite  Action = "db:write"
	ActionExport Action = "db:export"
	ActionAdmin  Action = "db:admin"
)

// Ability strings these actions map to today. They must equal the API's
// AbilityReadSensitive and AbilityRoot values; a test in the api package pins that.
const (
	abilityReadSensitive = "read:sensitive"
	abilityRoot          = "root"
)

// ResourcePrefix is the policy resource namespace for databases.
const ResourcePrefix = "database:"

// Actions lists every known db action, for policy validation.
var Actions = []Action{ActionRead, ActionQuery, ActionWrite, ActionExport, ActionAdmin}

// Known reports whether s names a db action.
func Known(s string) bool {
	for _, a := range Actions {
		if string(a) == s {
			return true
		}
	}
	return false
}

// EffectiveAbility is the flat ability an action needs today; unknown actions
// map to root so a mistake fails closed.
func EffectiveAbility(a Action) string {
	switch a {
	case ActionRead, ActionQuery, ActionExport:
		return abilityReadSensitive
	default:
		return abilityRoot
	}
}

// DenyNames lists the action names whose explicit Deny also blocks action:
// a Deny on db:read blocks query and export, a Deny on db:query blocks export.
// Write and admin Denies stay their own.
func DenyNames(a Action) []string {
	switch a {
	case ActionExport:
		return []string{string(ActionExport), string(ActionQuery), string(ActionRead)}
	case ActionQuery:
		return []string{string(ActionQuery), string(ActionRead)}
	default:
		return []string{string(a)}
	}
}

// needsStepUp lists actions that require a fresh step up when enforcement is on.
func needsStepUp(a Action) bool { return a == ActionWrite || a == ActionAdmin }

// Caller is the authenticated principal making the request.
type Caller struct {
	PrincipalType string
	PrincipalID   string
	Abilities     []string
	StepUpFresh   bool
}

// Verdict is the engine's single answer for one action on one resource.
type Verdict struct {
	Allowed  bool
	Denied   bool
	Hidden   bool
	ByAction bool // granted only by a policy Allow naming the db action
}

// PolicyEngine is the IAM evaluation the API supplies. One call applies
// visibility, team, environment and policy rules for the effective ability and
// the db action; an explicit Deny on either always wins.
type PolicyEngine interface {
	Evaluate(ctx context.Context, c Caller, ability string, action Action, resource string) (Verdict, error)
}

// Limits are per decision caps the handler applies; zero means engine default.
type Limits struct {
	MaxRows        int
	TimeoutSeconds int
}

// Resolver holds the engine and settings Resolve needs.
type Resolver struct {
	Engine          PolicyEngine
	Limits          Limits
	EnforceStepUp   bool
	LimitsForAction func(Action) Limits
}

// Decision is the outcome of Resolve.
type Decision struct {
	Allowed        bool
	Hidden         bool
	Reason         string
	Abilities      []string
	PolicyDecision string
	Limits         Limits
	RequiredStepUp bool
}

// Reasons and policy decision values.
const (
	ReasonOK           = "ok"
	ReasonBadInput     = "invalid action or resource"
	ReasonNoEngine     = "no policy engine"
	ReasonEngineError  = "policy evaluation failed"
	ReasonExplicitDeny = "explicit policy deny"
	ReasonNoAbility    = "missing ability"
	ReasonHidden       = "resource not visible"
	ReasonStepUp       = "step up required"

	PolicyNone    = "none"
	PolicyDenied  = "deny"
	PolicyAllowed = "allow"
)

// Resolve decides whether caller may perform action on resource ("database:name").
// Any error or doubt denies.
func (r *Resolver) Resolve(ctx context.Context, caller Caller, resource string, action Action) (Decision, error) {
	d := Decision{PolicyDecision: PolicyNone, RequiredStepUp: needsStepUp(action)}
	if !Known(string(action)) || !strings.HasPrefix(resource, ResourcePrefix) || len(resource) == len(ResourcePrefix) {
		d.Reason = ReasonBadInput
		return d, nil
	}
	if r == nil || r.Engine == nil {
		d.Reason = ReasonNoEngine
		return d, errors.New("dbauthz: no policy engine")
	}
	ability := EffectiveAbility(action)
	d.Abilities = []string{ability, string(action)}
	v, err := r.Engine.Evaluate(ctx, caller, ability, action, resource)
	if err != nil {
		d.Reason = ReasonEngineError
		return d, err
	}
	switch {
	case v.Hidden:
		d.Hidden, d.Reason = true, ReasonHidden
		return d, nil
	case v.Denied:
		d.PolicyDecision, d.Reason = PolicyDenied, ReasonExplicitDeny
		return d, nil
	case !v.Allowed:
		d.Reason = ReasonNoAbility
		return d, nil
	case v.ByAction:
		d.PolicyDecision = PolicyAllowed
	}
	if r.EnforceStepUp && d.RequiredStepUp && !caller.StepUpFresh {
		d.Reason = ReasonStepUp
		return d, nil
	}
	d.Allowed, d.Reason, d.Limits = true, ReasonOK, r.Limits
	if r.LimitsForAction != nil {
		d.Limits = r.LimitsForAction(action)
	}
	return d, nil
}
