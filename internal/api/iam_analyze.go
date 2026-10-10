package api

import (
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Finding kinds.
const (
	findingAllowAll            = "allow_all"
	findingWildcardAction      = "wildcard_action"
	findingSensitiveEverywhere = "sensitive_on_all_resources"
	findingDenyNeverMatches    = "deny_never_matches"
	findingDanglingResource    = "dangling_resource"
	findingUnusedPolicy        = "unused_policy"
	findingTokenRoot           = "token_with_root"
	findingOrphanAttachment    = "orphaned_attachment"
	findingAllowShadowed       = "allow_shadowed_by_deny"
	findingContradiction       = "contradictory_statements"
	findingDuplicate           = "duplicate_statement"
	findingUnsupportedKey      = "unsupported_key"
	findingMalformed           = "malformed_document"
)

// Finding severities, most serious first.
const (
	sevCritical = "critical"
	sevHigh     = "high"
	sevMedium   = "medium"
	sevLow      = "low"
	sevInfo     = "info"
)

var severityOrder = []string{sevCritical, sevHigh, sevMedium, sevLow, sevInfo}

var severityPenalty = map[string]int{sevCritical: 30, sevHigh: 15, sevMedium: 7, sevLow: 2, sevInfo: 0}

type finding struct {
	Kind           string `json:"kind"`
	Severity       string `json:"severity"`
	PolicyID       string `json:"policy_id,omitempty"`
	PolicyName     string `json:"policy_name,omitempty"`
	StatementIndex *int   `json:"statement_index,omitempty"`
	PrincipalType  string `json:"principal_type,omitempty"`
	PrincipalID    string `json:"principal_id,omitempty"`
	Message        string `json:"message"`
	Fix            string `json:"fix"`
}

type analysisResponse struct {
	Score    int            `json:"score"`
	Counts   map[string]int `json:"counts"`
	Findings []finding      `json:"findings"`
}

// patternCovers reports whether every value pattern b can match is also
// matched by pattern a, using the evaluator's trailing wildcard convention.
func patternCovers(a, b string) bool {
	if a == "*" || a == b {
		return true
	}
	if prefix, ok := strings.CutSuffix(a, "*"); ok {
		return strings.HasPrefix(b, prefix)
	}
	return false
}

func patternsOverlap(a, b string) bool {
	if patternCovers(a, b) || patternCovers(b, a) {
		return true
	}
	pa, aw := strings.CutSuffix(a, "*")
	pb, bw := strings.CutSuffix(b, "*")
	return aw && bw && (strings.HasPrefix(pa, pb) || strings.HasPrefix(pb, pa))
}

func listCovers(as, bs []string) bool {
	return len(bs) > 0 && !slices.ContainsFunc(bs, func(b string) bool {
		return !slices.ContainsFunc(as, func(a string) bool { return patternCovers(a, b) })
	})
}

func listOverlaps(as, bs []string) bool {
	return slices.ContainsFunc(as, func(a string) bool {
		return slices.ContainsFunc(bs, func(b string) bool { return patternsOverlap(a, b) })
	})
}

func stmtIndex(i int) *int { return &i }

func hasSensitiveAction(actions []string) bool {
	return slices.ContainsFunc(actions, func(a string) bool {
		return a == AbilityReadSensitive || a == AbilityWriteSensitive
	})
}

func describeStatement(i int, s Statement) string {
	return fmt.Sprintf("statement %d (%s %s on %s)", i+1, s.Effect, strings.Join(s.Action, ", "), strings.Join(s.Resource, ", "))
}

// analyzeDocument runs the per statement checks on one parsed document.
func analyzeDocument(policyID, policyName string, doc *Document, inv *iamInventory) []finding {
	var out []finding
	add := func(i int, kind, sev, msg, fix string) {
		out = append(out, finding{Kind: kind, Severity: sev, PolicyID: policyID, PolicyName: policyName, StatementIndex: stmtIndex(i), Message: msg, Fix: fix})
	}
	for i, s := range doc.Statement {
		desc := describeStatement(i, s)
		allActions := slices.Contains(s.Action, "*")
		allResources := slices.Contains(s.Resource, "*")
		if s.Effect == EffectAllow {
			switch {
			case allActions && allResources:
				add(i, findingAllowAll, sevCritical, desc+" allows everything on everything, with no condition to narrow it.", "Name the abilities and the resources this policy is for.")
			case allActions || slices.Contains(s.Action, AbilityRoot):
				add(i, findingWildcardAction, sevHigh, desc+" grants every ability or root.", "List only the abilities this principal needs.")
			case hasSensitiveAction(s.Action) && allResources:
				add(i, findingSensitiveEverywhere, sevMedium, desc+" grants a sensitive ability on every resource.", "Scope the resources to the apps or databases that need it.")
			}
		}
		for j, other := range doc.Statement {
			if j <= i {
				continue
			}
			out = append(out, comparePair(policyID, policyName, i, s, j, other)...)
		}
		if inv != nil {
			out = append(out, danglingResourceFindings(policyID, policyName, i, s, desc, inv)...)
		}
	}
	if policyName != "" {
		for i := range out {
			out[i].Message = "Policy " + policyName + ": " + out[i].Message
		}
	}
	return out
}

func comparePair(policyID, policyName string, i int, a Statement, j int, b Statement) []finding {
	mk := func(kind, sev, msg, fix string) finding {
		return finding{Kind: kind, Severity: sev, PolicyID: policyID, PolicyName: policyName, StatementIndex: stmtIndex(j), Message: msg, Fix: fix}
	}
	if a.Effect == b.Effect {
		if listCovers(a.Action, b.Action) && listCovers(a.Resource, b.Resource) && listCovers(b.Action, a.Action) && listCovers(b.Resource, a.Resource) {
			return []finding{mk(findingDuplicate, sevLow, fmt.Sprintf("Statements %d and %d say the same thing.", i+1, j+1), "Remove one of them.")}
		}
		return nil
	}
	allow, deny, allowIdx, denyIdx := a, b, i, j
	if a.Effect == EffectDeny {
		allow, deny, allowIdx, denyIdx = b, a, j, i
	}
	switch {
	case listCovers(deny.Action, allow.Action) && listCovers(deny.Resource, allow.Resource):
		return []finding{mk(findingAllowShadowed, sevMedium, fmt.Sprintf("Allow statement %d can never take effect: Deny statement %d covers all of it.", allowIdx+1, denyIdx+1), "Remove the Allow, or narrow the Deny.")}
	case listOverlaps(allow.Action, deny.Action) && listOverlaps(allow.Resource, deny.Resource):
		return []finding{mk(findingContradiction, sevInfo, fmt.Sprintf("Allow statement %d and Deny statement %d overlap. Deny wins where they do.", allowIdx+1, denyIdx+1), "Check the overlap is intended, or split the resources.")}
	}
	return nil
}

func danglingResourceFindings(policyID, policyName string, i int, s Statement, desc string, inv *iamInventory) []finding {
	var out []finding
	for _, r := range s.Resource {
		if strings.Contains(r, "*") || inv.resourceExists(r) {
			continue
		}
		kind, sev, msg := findingDanglingResource, sevLow, fmt.Sprintf("%s refers to %s, which no longer exists.", desc, r)
		fix := "Remove " + r + " from the statement."
		if s.Effect == EffectDeny {
			kind, sev = findingDenyNeverMatches, sevMedium
			msg = fmt.Sprintf("%s refers to %s, which does not exist, so this Deny can never match.", desc, r)
		}
		out = append(out, finding{Kind: kind, Severity: sev, PolicyID: policyID, PolicyName: policyName, StatementIndex: stmtIndex(i), Message: msg, Fix: fix})
	}
	return out
}

// unsupportedKeyFindings flags statement keys the evaluator never reads.
func unsupportedKeyFindings(p store.Policy) []finding {
	issues := validateDocumentFields(p.Document, nil)
	var out []finding
	for _, is := range issues {
		if is.Severity != issueWarning {
			continue
		}
		out = append(out, finding{Kind: findingUnsupportedKey, Severity: sevHigh, PolicyID: p.ID, PolicyName: p.Name, Message: "Policy " + p.Name + ": " + is.Path + ": " + is.Message, Fix: "Remove the key, the evaluator ignores it."})
	}
	return out
}

func scoreFindings(fs []finding) analysisResponse {
	counts := map[string]int{}
	score := 100
	for _, f := range fs {
		counts[f.Severity]++
		score -= severityPenalty[f.Severity]
	}
	sort.SliceStable(fs, func(i, j int) bool {
		return slices.Index(severityOrder, fs[i].Severity) < slices.Index(severityOrder, fs[j].Severity)
	})
	if fs == nil {
		fs = []finding{}
	}
	return analysisResponse{Score: max(score, 0), Counts: counts, Findings: fs}
}

// analyzePlatform runs every check over stored policies, attachments and
// principals.
func (rt *Router) analyzePlatform(r *http.Request) (analysisResponse, error) {
	ctx := r.Context()
	policies, err := rt.policies.ListPolicies(ctx)
	if err != nil {
		return analysisResponse{}, fmt.Errorf("analyze: list policies: %w", err)
	}
	inv, err := rt.loadIAMInventory(ctx)
	if err != nil {
		return analysisResponse{}, err
	}
	principals, err := rt.loadIAMPrincipals(ctx)
	if err != nil {
		return analysisResponse{}, err
	}
	var out []finding
	byPrincipal := map[string][]store.Policy{}
	for _, p := range policies {
		doc, perr := ParseDocument(p.Document)
		if perr != nil {
			out = append(out, finding{Kind: findingMalformed, Severity: sevHigh, PolicyID: p.ID, PolicyName: p.Name, Message: "The stored document does not parse, so the evaluator ignores this policy: " + perr.Error(), Fix: "Edit the policy and save a valid document."})
			continue
		}
		out = append(out, analyzeDocument(p.ID, p.Name, doc, inv)...)
		out = append(out, unsupportedKeyFindings(p)...)
		atts, aerr := rt.policies.ListAttachmentsForPolicy(ctx, p.ID)
		if aerr != nil {
			return analysisResponse{}, fmt.Errorf("analyze: list attachments for %s: %w", p.ID, aerr)
		}
		if len(atts) == 0 {
			out = append(out, finding{Kind: findingUnusedPolicy, Severity: sevLow, PolicyID: p.ID, PolicyName: p.Name, Message: "Policy " + p.Name + " is not attached to anyone.", Fix: "Attach it to a user or token, or delete it."})
		}
		for _, a := range atts {
			if _, ok := findPrincipal(principals, a.PrincipalType, a.PrincipalID); !ok {
				out = append(out, finding{Kind: findingOrphanAttachment, Severity: sevLow, PolicyID: p.ID, PolicyName: p.Name, PrincipalType: a.PrincipalType, PrincipalID: a.PrincipalID, Message: fmt.Sprintf("Policy %s is attached to a %s that no longer exists.", p.Name, a.PrincipalType), Fix: "Detach the policy from the missing " + a.PrincipalType + "."})
				continue
			}
			key := a.PrincipalType + "/" + a.PrincipalID
			byPrincipal[key] = append(byPrincipal[key], p)
		}
	}
	out = append(out, crossPolicyFindings(byPrincipal, principals)...)
	for _, p := range principals {
		if p.Type == store.PrincipalTypeToken && p.Active && p.isRoot() && p.Name != AIAssistantTokenName {
			out = append(out, finding{Kind: findingTokenRoot, Severity: sevMedium, PrincipalType: p.Type, PrincipalID: p.ID, Message: "Token " + p.Name + " holds root, which no policy can narrow for platform routes.", Fix: "Mint a scoped token for this job and revoke this one."})
		}
	}
	return scoreFindings(out), nil
}

// crossPolicyFindings compares statements across different policies attached
// to the same principal, where an Allow and a Deny meet.
func crossPolicyFindings(byPrincipal map[string][]store.Policy, principals []iamPrincipal) []finding {
	var out []finding
	keys := make([]string, 0, len(byPrincipal))
	for k := range byPrincipal {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	seen := map[string]bool{}
	for _, k := range keys {
		pols := byPrincipal[k]
		for i := range pols {
			for j := range pols {
				if i == j {
					continue
				}
				if f, ok := crossPair(pols[i], pols[j], k, principals); ok && !seen[f.PolicyID+f.Message] {
					seen[f.PolicyID+f.Message] = true
					out = append(out, f)
				}
			}
		}
	}
	return out
}

func crossPair(allowPol, denyPol store.Policy, key string, principals []iamPrincipal) (finding, bool) {
	ad, aerr := ParseDocument(allowPol.Document)
	dd, derr := ParseDocument(denyPol.Document)
	if aerr != nil || derr != nil {
		return finding{}, false
	}
	for _, a := range ad.Statement {
		if a.Effect != EffectAllow {
			continue
		}
		for _, d := range dd.Statement {
			if d.Effect == EffectDeny && listCovers(d.Action, a.Action) && listCovers(d.Resource, a.Resource) {
				ptype, pid, _ := strings.Cut(key, "/")
				name := pid
				if p, ok := findPrincipal(principals, ptype, pid); ok {
					name = p.Name
				}
				return finding{Kind: findingAllowShadowed, Severity: sevMedium, PolicyID: allowPol.ID, PolicyName: allowPol.Name, PrincipalType: ptype, PrincipalID: pid,
					Message: fmt.Sprintf("For %s, policy %s allows what policy %s denies, so that Allow never takes effect.", name, allowPol.Name, denyPol.Name),
					Fix:     "Detach one of the two policies from " + name + ", or narrow the Deny."}, true
			}
		}
	}
	return finding{}, false
}

// handleIAMAnalyze handles GET /api/v1/iam/analyze.
func (rt *Router) handleIAMAnalyze(w http.ResponseWriter, r *http.Request) {
	res, err := rt.analyzePlatform(r)
	if err != nil {
		rt.internalError(w, "api: iam analyze failed", err, slog.String("path", r.URL.Path))
		return
	}
	writeJSON(w, http.StatusOK, res)
}
