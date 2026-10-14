package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/accesspolicy"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	minIdleTimeout     = 5 * time.Minute
	minMaxLifetime     = 15 * time.Minute
	maxPolicyDuration  = 90 * 24 * time.Hour
	maxAllowlistLength = 100
)

type sessionPolicyResource struct {
	TeamID      string   `json:"team_id"`
	TeamName    string   `json:"team_name"`
	IdleTimeout string   `json:"idle_timeout"`
	MaxLifetime string   `json:"max_lifetime"`
	IPAllowlist []string `json:"ip_allowlist"`
	RequireMFA  bool     `json:"require_mfa"`
	UpdatedBy   string   `json:"updated_by"`
	UpdatedAt   string   `json:"updated_at"`
}

type sessionPolicyRequest struct {
	IdleTimeout string   `json:"idle_timeout"`
	MaxLifetime string   `json:"max_lifetime"`
	IPAllowlist []string `json:"ip_allowlist"`
	RequireMFA  bool     `json:"require_mfa"`
}

type effectivePolicyResource struct {
	IdleTimeout string `json:"idle_timeout"`
	MaxLifetime string `json:"max_lifetime"`
	RequireMFA  bool   `json:"require_mfa"`
	AllowLists  int    `json:"allowlists"`
	YourAddress string `json:"your_address"`
	Allowed     bool   `json:"address_allowed"`
}

func formatPolicyDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}

func (rt *Router) toSessionPolicyResource(r *http.Request, p store.SessionPolicy) sessionPolicyResource {
	name := p.TeamID
	if org, err := rt.organizations.GetOrganization(r.Context(), p.TeamID); err == nil {
		name = org.Name
	}
	return sessionPolicyResource{TeamID: p.TeamID, TeamName: name, IdleTimeout: formatPolicyDuration(p.IdleTimeout),
		MaxLifetime: formatPolicyDuration(p.MaxLifetime), IPAllowlist: nonNilStrings(p.IPAllowlist), RequireMFA: p.RequireMFA,
		UpdatedBy: p.UpdatedBy, UpdatedAt: p.UpdatedAt.UTC().Format(time.RFC3339)}
}

func (rt *Router) accessPolicyReady(w http.ResponseWriter) bool {
	if rt.accessPolicy == nil {
		writeError(w, http.StatusNotImplemented, "session policies are not available on this control plane")
		return false
	}
	return true
}

func (rt *Router) handleListSessionPolicies(w http.ResponseWriter, r *http.Request) {
	if !rt.accessPolicyReady(w) {
		return
	}
	ps, err := rt.accessPolicy.ListSessionPolicies(r.Context())
	if err != nil {
		rt.internalError(w, "api: list session policies failed", err)
		return
	}
	out := make([]sessionPolicyResource, 0, len(ps))
	for _, p := range ps {
		out = append(out, rt.toSessionPolicyResource(r, p))
	}
	writeJSON(w, http.StatusOK, out)
}

func parsePolicyDuration(field, raw string, minimum time.Duration) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < minimum || d > maxPolicyDuration {
		return 0, errors.New(field + " must be empty or a duration between " + minimum.String() + " and 2160h")
	}
	return d, nil
}

func parseSessionPolicyRequest(teamID string, req sessionPolicyRequest) (store.SessionPolicy, error) {
	idle, err := parsePolicyDuration("idle_timeout", req.IdleTimeout, minIdleTimeout)
	if err != nil {
		return store.SessionPolicy{}, err
	}
	life, err := parsePolicyDuration("max_lifetime", req.MaxLifetime, minMaxLifetime)
	if err != nil {
		return store.SessionPolicy{}, err
	}
	if len(req.IPAllowlist) > maxAllowlistLength {
		return store.SessionPolicy{}, errors.New("ip_allowlist holds at most 100 entries")
	}
	prefixes, err := accesspolicy.ParseAllowlist(req.IPAllowlist)
	if err != nil {
		return store.SessionPolicy{}, err
	}
	list := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		list = append(list, p.String())
	}
	return store.SessionPolicy{TeamID: teamID, IdleTimeout: idle, MaxLifetime: life, IPAllowlist: list, RequireMFA: req.RequireMFA}, nil
}

// lockoutCheck refuses a policy that would shut out the admin saving it: the
// caller's own address must pass and, when MFA becomes required, they need a factor.
func (rt *Router) lockoutCheck(r *http.Request, next store.SessionPolicy) error {
	sub := rt.subjectOf(r)
	if sub.userID == "" {
		return nil
	}
	ctx := r.Context()
	stored, err := rt.accessPolicy.SessionPoliciesForUser(ctx, sub.userID)
	if err != nil {
		return err
	}
	member := false
	for i := range stored {
		if stored[i].TeamID == next.TeamID {
			stored[i], member = next, true
		}
	}
	if !member {
		if _, err := rt.accessPolicy.TeamMemberRole(ctx, next.TeamID, sub.userID); err == nil {
			stored = append(stored, next)
		}
	}
	rules, err := accesspolicy.Merge(toPolicies(stored))
	if err != nil {
		return err
	}
	addr := policyClientAddr(r)
	if !rules.IPAllowed(addr) {
		return errors.New("this policy would block your own address " + addr.String() + ": add it to the allowlist first")
	}
	if rules.RequireMFA && sub.sessionID != "" {
		ok, err := rt.hasSecondFactor(ctx, sub.userID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("enroll a second factor on your own account before requiring one")
		}
	}
	return nil
}

func (rt *Router) handlePutSessionPolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.accessPolicyReady(w) {
		return
	}
	teamID := r.PathValue("team_id")
	if _, err := rt.organizations.GetOrganization(r.Context(), teamID); err != nil {
		writeError(w, http.StatusNotFound, "team not found")
		return
	}
	var req sessionPolicyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	p, err := parseSessionPolicyRequest(teamID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := rt.lockoutCheck(r, p); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	_, actorID, actor, _ := rt.currentActor(r)
	if actor == "" {
		actor = actorID
	}
	p.UpdatedBy, p.UpdatedAt = actor, time.Now()
	if err := rt.accessPolicy.SaveSessionPolicy(r.Context(), p); err != nil {
		rt.internalError(w, "api: save session policy failed", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.toSessionPolicyResource(r, p))
}

func (rt *Router) handleDeleteSessionPolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.accessPolicyReady(w) {
		return
	}
	err := rt.accessPolicy.DeleteSessionPolicy(r.Context(), r.PathValue("team_id"))
	if errors.Is(err, store.ErrSessionPolicyNotFound) {
		writeError(w, http.StatusNotFound, "no session policy for that team")
		return
	}
	if err != nil {
		rt.internalError(w, "api: delete session policy failed", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleEffectiveSessionPolicy shows the caller the rules that apply to them.
func (rt *Router) handleEffectiveSessionPolicy(w http.ResponseWriter, r *http.Request) {
	if !rt.accessPolicyReady(w) {
		return
	}
	sub := rt.subjectOf(r)
	rules, err := rt.effectiveRules(r.Context(), sub.userID)
	if err != nil {
		rt.internalError(w, "api: effective session policy failed", err)
		return
	}
	addr := policyClientAddr(r)
	writeJSON(w, http.StatusOK, effectivePolicyResource{IdleTimeout: formatPolicyDuration(rules.IdleTimeout),
		MaxLifetime: formatPolicyDuration(rules.MaxLifetime), RequireMFA: rules.RequireMFA, AllowLists: len(rules.AllowLists),
		YourAddress: addr.String(), Allowed: rules.IPAllowed(addr)})
}
