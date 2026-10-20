package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	usageWindow       = 7 * 24 * time.Hour
	deltaNameLimit    = 50
	guardProbeName    = "iam-guard-probe"
	newPolicyIDMarker = "new"
)

// errWouldRemoveLastRoot is the guard's refusal.
var errWouldRemoveLastRoot = errors.New("this change would leave no root principal able to manage the platform")

type principalRef struct {
	Type string `json:"principal_type"`
	ID   string `json:"principal_id"`
}

func (p principalRef) key() string { return p.Type + "/" + p.ID }

// policyOverlay describes a hypothetical change: replace one policy's
// document, attach it to some principals, detach it from others.
type policyOverlay struct {
	policy   store.Policy
	replace  bool
	attached map[string]bool
	attach   map[string]bool
	detach   map[string]bool
}

func (o policyOverlay) apply(key string, current []store.Policy) []store.Policy {
	out := make([]store.Policy, 0, len(current)+1)
	has := false
	for _, p := range current {
		if o.policy.ID != "" && p.ID == o.policy.ID {
			has = true
			if o.detach[key] {
				continue
			}
			if o.replace {
				p.Document = o.policy.Document
			}
		}
		out = append(out, p)
	}
	if !has && o.attach[key] {
		out = append(out, o.policy)
	}
	return out
}

func (o policyOverlay) affects(key string) bool {
	return o.attach[key] || o.detach[key] || (o.replace && o.attached[key])
}

type resourceDelta struct {
	Ability   string   `json:"ability"`
	Risk      string   `json:"risk"`
	Count     int      `json:"count"`
	Apps      []string `json:"apps"`
	Databases []string `json:"databases"`
}

type principalChange struct {
	Principal    iamPrincipal    `json:"principal"`
	Gains        []resourceDelta `json:"gains"`
	Losses       []resourceDelta `json:"losses"`
	RecentlyUsed bool            `json:"recently_used"`
}

type guardResult struct {
	Blocked bool   `json:"blocked"`
	Reason  string `json:"reason,omitempty"`
}

type previewResponse struct {
	Valid     bool              `json:"valid"`
	Issues    []fieldIssue      `json:"issues"`
	Changes   []principalChange `json:"changes"`
	Guard     guardResult       `json:"guard"`
	UsageNote string            `json:"usage_note"`
}

type previewRequest struct {
	PolicyID string          `json:"policy_id"`
	Document json.RawMessage `json:"document"`
	Attach   []principalRef  `json:"attach"`
	Detach   []principalRef  `json:"detach"`
}

const usageNoteText = "Usage is tracked per token as a last used time, not per policy, so this shows who used the platform recently, not who relied on this policy."

func diffNames(before, after []string) (gained, lost []string) {
	for _, n := range after {
		if !slices.Contains(before, n) {
			gained = append(gained, n)
		}
	}
	for _, n := range before {
		if !slices.Contains(after, n) {
			lost = append(lost, n)
		}
	}
	return gained, lost
}

func trimNames(in []string) []string {
	if in == nil {
		return []string{}
	}
	if len(in) > deltaNameLimit {
		return in[:deltaNameLimit]
	}
	return in
}

// diffEffective turns two effective permission sets into gains and losses.
func diffEffective(before, after []effectiveAbility) (gains, losses []resourceDelta) {
	gains, losses = []resourceDelta{}, []resourceDelta{}
	for i := range after {
		ga, la := diffNames(before[i].Apps, after[i].Apps)
		gd, ld := diffNames(before[i].Databases, after[i].Databases)
		if n := len(ga) + len(gd); n > 0 {
			gains = append(gains, resourceDelta{Ability: after[i].Ability, Risk: after[i].Risk, Count: n, Apps: trimNames(ga), Databases: trimNames(gd)})
		}
		if n := len(la) + len(ld); n > 0 {
			losses = append(losses, resourceDelta{Ability: after[i].Ability, Risk: after[i].Risk, Count: n, Apps: trimNames(la), Databases: trimNames(ld)})
		}
	}
	return gains, losses
}

// rootOperational reports whether a root principal can still use root on the
// platform and on every app and database it has not been named in a Deny for.
func rootOperational(p iamPrincipal, policies []store.Policy) bool {
	if !p.Active || !p.isRoot() {
		return false
	}
	for _, res := range []string{"*", resourcePrefixApp + guardProbeName, resourcePrefixDatabase + guardProbeName} {
		if !authorizeResource(p.Abilities, policies, AbilityRoot, res) {
			return false
		}
	}
	return true
}

// buildOverlay resolves a request into an overlay and its affected principals.
func (rt *Router) buildOverlay(ctx context.Context, req previewRequest) (policyOverlay, error) {
	o := policyOverlay{attached: map[string]bool{}, attach: map[string]bool{}, detach: map[string]bool{}}
	if req.PolicyID != "" {
		p, err := rt.policies.GetPolicy(ctx, req.PolicyID)
		if errors.Is(err, store.ErrPolicyNotFound) {
			return o, err
		}
		if err != nil {
			return o, fmt.Errorf("overlay: get policy: %w", err)
		}
		o.policy = *p
		atts, err := rt.policies.ListAttachmentsForPolicy(ctx, req.PolicyID)
		if err != nil {
			return o, fmt.Errorf("overlay: list attachments: %w", err)
		}
		for _, a := range atts {
			o.attached[principalRef{a.PrincipalType, a.PrincipalID}.key()] = true
		}
	} else {
		o.policy = store.Policy{ID: newPolicyIDMarker, Name: newPolicyIDMarker}
	}
	if len(req.Document) > 0 {
		o.policy.Document, o.replace = string(req.Document), true
	}
	for _, a := range req.Attach {
		o.attach[a.key()] = true
	}
	for _, d := range req.Detach {
		o.detach[d.key()] = true
	}
	return o, nil
}

// checkRootGuard refuses a change that takes the last operational root
// principal away. A platform with no operational root before the change is not
// blocked, since the change cannot make it worse.
func (rt *Router) checkRootGuard(ctx context.Context, o policyOverlay, principals []iamPrincipal) (guardResult, error) {
	before, after := 0, 0
	for _, p := range principals {
		if !p.Active || !p.isRoot() {
			continue
		}
		cur, err := rt.policies.ListPoliciesForPrincipal(ctx, p.Type, p.ID)
		if err != nil {
			return guardResult{}, fmt.Errorf("guard: list policies for %s: %w", p.ID, err)
		}
		if rootOperational(p, cur) {
			before++
		}
		if rootOperational(p, o.apply(principalRef{p.Type, p.ID}.key(), cur)) {
			after++
		}
	}
	if before > 0 && after == 0 {
		return guardResult{Blocked: true, Reason: errWouldRemoveLastRoot.Error()}, nil
	}
	return guardResult{}, nil
}

// enforceRootGuard writes a 409 and returns true when the change is refused.
func (rt *Router) enforceRootGuard(w http.ResponseWriter, r *http.Request, req previewRequest) bool {
	ctx := r.Context()
	o, err := rt.buildOverlay(ctx, req)
	if errors.Is(err, store.ErrPolicyNotFound) {
		writeError(w, http.StatusNotFound, errPolicyNotFound)
		return true
	}
	if err != nil {
		rt.internalError(w, "api: root guard: build overlay failed", err, slog.String("policy_id", req.PolicyID))
		return true
	}
	principals, err := rt.loadIAMPrincipals(ctx)
	if err != nil {
		rt.internalError(w, "api: root guard: load principals failed", err)
		return true
	}
	g, err := rt.checkRootGuard(ctx, o, principals)
	if err != nil {
		rt.internalError(w, "api: root guard failed", err, slog.String("policy_id", req.PolicyID))
		return true
	}
	if g.Blocked {
		writeError(w, http.StatusConflict, g.Reason)
		return true
	}
	return false
}

// previewChanges computes each affected principal's before and after.
func (rt *Router) previewChanges(ctx context.Context, o policyOverlay, principals []iamPrincipal, inv *iamInventory) ([]principalChange, error) {
	out := []principalChange{}
	now := time.Now()
	for _, p := range principals {
		key := principalRef{p.Type, p.ID}.key()
		if !o.affects(key) {
			continue
		}
		cur, err := rt.policies.ListPoliciesForPrincipal(ctx, p.Type, p.ID)
		if err != nil {
			return nil, fmt.Errorf("preview: list policies for %s: %w", p.ID, err)
		}
		before, _, err := rt.effectiveFor(ctx, p, cur, inv)
		if err != nil {
			return nil, err
		}
		after, _, err := rt.effectiveFor(ctx, p, o.apply(key, cur), inv)
		if err != nil {
			return nil, err
		}
		gains, losses := diffEffective(before, after)
		used := p.LastUsedAt != nil && now.Sub(*p.LastUsedAt) < usageWindow
		out = append(out, principalChange{Principal: p, Gains: gains, Losses: losses, RecentlyUsed: used})
	}
	return out, nil
}

// handleIAMPreview handles POST /api/v1/iam/preview: what a change would do
// before it is saved. It persists nothing.
func (rt *Router) handleIAMPreview(w http.ResponseWriter, r *http.Request) {
	var req previewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, errInvalidPolicyRequestBody)
		return
	}
	ctx := r.Context()
	inv, err := rt.loadIAMInventory(ctx)
	if err != nil {
		rt.internalError(w, "api: iam preview: inventory failed", err)
		return
	}
	resp := previewResponse{Valid: true, Issues: []fieldIssue{}, Changes: []principalChange{}, UsageNote: usageNoteText}
	if len(req.Document) > 0 {
		resp.Issues = validateDocumentFields(string(req.Document), inv)
		if resp.Issues == nil {
			resp.Issues = []fieldIssue{}
		}
		if resp.Valid = !hasErrorIssue(resp.Issues); !resp.Valid {
			writeJSON(w, http.StatusOK, resp)
			return
		}
	}
	o, err := rt.buildOverlay(ctx, req)
	if errors.Is(err, store.ErrPolicyNotFound) {
		writeError(w, http.StatusNotFound, errPolicyNotFound)
		return
	}
	if err != nil {
		rt.internalError(w, "api: iam preview: overlay failed", err, slog.String("policy_id", req.PolicyID))
		return
	}
	principals, err := rt.loadIAMPrincipals(ctx)
	if err != nil {
		rt.internalError(w, "api: iam preview: principals failed", err)
		return
	}
	if resp.Changes, err = rt.previewChanges(ctx, o, principals, inv); err != nil {
		rt.internalError(w, "api: iam preview failed", err, slog.String("policy_id", req.PolicyID))
		return
	}
	if resp.Guard, err = rt.checkRootGuard(ctx, o, principals); err != nil {
		rt.internalError(w, "api: iam preview: guard failed", err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
