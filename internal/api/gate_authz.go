package api

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Reasons recorded with gate decisions.
const (
	gateReasonNoRules         = "no_rules"
	gateReasonNotAllowed      = "not_allowed"
	gateReasonUserRemoved     = "user_removed"
	gateReasonEnvNotGranted   = "environment_not_granted"
	gateReasonMatchUser       = "user_rule"
	gateReasonMatchTeam       = "team_rule"
	gateReasonMatchRole       = "role_rule"
	gateReasonNotBuiltin      = "gate_not_builtin"
	gateReasonHTTPSRequired   = "https_required"
	gateReasonNoSession       = "no_session"
	gateReasonSessionRevoked  = "revoked"
	gateReasonSessionExpired  = "expired"
	gateReasonSignedOut       = "signed_out"
	gateReasonWrongHost       = "wrong_host"
	gateReasonTokenInvalid    = "token_invalid"
	gateReasonTokenExpired    = "token_expired"
	gateReasonStateMismatch   = "state_mismatch"
	maxGateRules              = 100
	maxGateRuleSubjectLen     = 320
	gateRuleSubjectNotAllowed = "\r\n\t"
)

// authorizeGateUser decides whether user may pass domain's built-in gate.
// No rules means nobody passes. A guest whose role only sees granted
// environments also needs a grant for the app's environment.
func (rt *Router) authorizeGateUser(ctx context.Context, domain string, user *store.User) (bool, string, error) {
	rules, err := rt.gate.ListGateRules(ctx, domain)
	if err != nil {
		return false, "", fmt.Errorf("api: gate rules: %w", err)
	}
	if len(rules) == 0 {
		return false, gateReasonNoRules, nil
	}
	reason, err := rt.matchGateRules(ctx, rules, user.ID)
	if err != nil || reason == "" {
		return false, gateReasonNotAllowed, err
	}
	vis, err := rt.gate.UserVisibility(ctx, user.ID)
	if errors.Is(err, store.ErrUserNotFound) {
		return false, gateReasonUserRemoved, nil
	}
	if err != nil {
		return false, "", fmt.Errorf("api: gate visibility: %w", err)
	}
	if vis.Restricted {
		env, err := rt.gate.DomainEnvironmentID(ctx, domain)
		if err != nil {
			return false, "", fmt.Errorf("api: gate environment: %w", err)
		}
		if !vis.Allows(env) {
			return false, gateReasonEnvNotGranted, nil
		}
	}
	return true, reason, nil
}

// matchGateRules returns the reason of the first matching rule, or "".
func (rt *Router) matchGateRules(ctx context.Context, rules []store.GateRule, userID string) (string, error) {
	var teams map[string]string
	var roleID string
	var teamsLoaded, roleLoaded bool
	for _, rule := range rules {
		switch rule.Kind {
		case store.GateRuleUser:
			if rule.Subject == userID {
				return gateReasonMatchUser, nil
			}
		case store.GateRuleTeam:
			if !teamsLoaded {
				var err error
				if teams, err = rt.gate.UserTeamRoles(ctx, userID); err != nil {
					return "", fmt.Errorf("api: gate team roles: %w", err)
				}
				teamsLoaded = true
			}
			if _, ok := teams[rule.Subject]; ok {
				return gateReasonMatchTeam, nil
			}
		case store.GateRuleRole:
			if !roleLoaded {
				var err error
				if roleID, err = rt.gate.UserRoleID(ctx, userID); err != nil && !errors.Is(err, store.ErrUserNotFound) {
					return "", fmt.Errorf("api: gate role: %w", err)
				}
				roleLoaded = true
			}
			if roleID != "" && rule.Subject == roleID {
				return gateReasonMatchRole, nil
			}
		}
	}
	return "", nil
}

// gateRuleError is an operator mistake in an allow list (a 400), as
// opposed to a store failure.
type gateRuleError struct{ msg string }

func (e *gateRuleError) Error() string { return e.msg }

func gateRuleErrorf(format string, args ...any) error {
	return &gateRuleError{msg: fmt.Sprintf(format, args...)}
}

// gateRuleInput is one entry of a PUT's allow list. Subject is an email or
// user id, a team id or name, or a role id or name.
type gateRuleInput struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
}

// gateRuleResource is a stored rule with a display label.
type gateRuleResource struct {
	Kind    string `json:"kind"`
	Subject string `json:"subject"`
	Label   string `json:"label"`
}

// resolveGateRules turns operator input into stored rules, rejecting
// subjects that do not exist so a typo never silently grants nothing.
func (rt *Router) resolveGateRules(ctx context.Context, in []gateRuleInput) ([]store.GateRule, error) {
	if len(in) > maxGateRules {
		return nil, gateRuleErrorf("allow accepts at most %d entries", maxGateRules)
	}
	out := make([]store.GateRule, 0, len(in))
	for _, r := range in {
		subject := strings.TrimSpace(r.Subject)
		if subject == "" || len(subject) > maxGateRuleSubjectLen || strings.ContainsAny(subject, gateRuleSubjectNotAllowed) {
			return nil, gateRuleErrorf("allow: %q is not a valid subject", r.Subject)
		}
		var id string
		var err error
		switch r.Kind {
		case store.GateRuleUser:
			id, err = rt.resolveGateUser(ctx, subject)
		case store.GateRuleTeam:
			id, err = rt.resolveGateTeam(ctx, subject)
		case store.GateRuleRole:
			id, err = rt.resolveGateRole(ctx, subject)
		default:
			return nil, gateRuleErrorf("allow: kind must be user, team or role, not %q", r.Kind)
		}
		if err != nil {
			return nil, err
		}
		out = append(out, store.GateRule{Kind: r.Kind, Subject: id})
	}
	return out, nil
}

func (rt *Router) resolveGateUser(ctx context.Context, subject string) (string, error) {
	var u *store.User
	var err error
	if strings.Contains(subject, "@") {
		u, err = rt.gate.GetUserByEmail(ctx, subject)
		if lower := strings.ToLower(subject); errors.Is(err, store.ErrUserNotFound) && lower != subject {
			u, err = rt.gate.GetUserByEmail(ctx, lower)
		}
	} else {
		u, err = rt.gate.GetUserByID(ctx, subject)
	}
	if errors.Is(err, store.ErrUserNotFound) {
		return "", gateRuleErrorf("allow: no user %q on this installation", subject)
	}
	if err != nil {
		return "", fmt.Errorf("api: resolve gate user: %w", err)
	}
	return u.ID, nil
}

func (rt *Router) resolveGateTeam(ctx context.Context, subject string) (string, error) {
	if o, err := rt.gate.GetOrganization(ctx, subject); err == nil {
		return o.ID, nil
	} else if !errors.Is(err, store.ErrOrganizationNotFound) {
		return "", fmt.Errorf("api: resolve gate team: %w", err)
	}
	orgs, err := rt.gate.ListOrganizations(ctx)
	if err != nil {
		return "", fmt.Errorf("api: list teams: %w", err)
	}
	var match []string
	for _, o := range orgs {
		if strings.EqualFold(o.Name, subject) {
			match = append(match, o.ID)
		}
	}
	switch len(match) {
	case 0:
		return "", gateRuleErrorf("allow: no team %q", subject)
	case 1:
		return match[0], nil
	default:
		return "", gateRuleErrorf("allow: more than one team is named %q, use its id", subject)
	}
}

func (rt *Router) resolveGateRole(ctx context.Context, subject string) (string, error) {
	if r, err := rt.gate.GetRole(ctx, subject); err == nil {
		return r.ID, nil
	} else if !errors.Is(err, store.ErrRoleNotFound) {
		return "", fmt.Errorf("api: resolve gate role: %w", err)
	}
	roles, err := rt.gate.ListRoles(ctx)
	if err != nil {
		return "", fmt.Errorf("api: list roles: %w", err)
	}
	for _, r := range roles {
		if strings.EqualFold(r.Name, subject) {
			return r.ID, nil
		}
	}
	return "", gateRuleErrorf("allow: no role %q", subject)
}

// gateRuleResources labels stored rules for display; a subject that no
// longer resolves keeps an empty label.
func (rt *Router) gateRuleResources(ctx context.Context, rules []store.GateRule) []gateRuleResource {
	out := make([]gateRuleResource, 0, len(rules))
	for _, r := range rules {
		res := gateRuleResource{Kind: r.Kind, Subject: r.Subject}
		switch r.Kind {
		case store.GateRuleUser:
			if u, err := rt.gate.GetUserByID(ctx, r.Subject); err == nil {
				res.Label = u.Email
			}
		case store.GateRuleTeam:
			if o, err := rt.gate.GetOrganization(ctx, r.Subject); err == nil {
				res.Label = o.Name
			}
		case store.GateRuleRole:
			if role, err := rt.gate.GetRole(ctx, r.Subject); err == nil {
				res.Label = role.Name
			}
		}
		out = append(out, res)
	}
	return out
}
