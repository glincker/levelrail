package api

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Counts are pinned so adding a mutating route forces a human look at whether it is audited.
const (
	pinnedNonGETRoutes   = 1016
	pinnedExemptions     = 116
	pinnedKnownAuditGaps = 0
)

var restRouteDeclRe = regexp.MustCompile(`mux\.Handle(?:Func)?\("(POST|PUT|PATCH|DELETE)\s*([^"]*)"(?:\s*\+\s*(\w+)(?:\s*\+\s*"([^"]*)")?)?,\s*(.*)$`)

var (
	restAbilityRe     = regexp.MustCompile(`require(?:AbilityForResource|AbilityDecided|Ability|AssetBucket)\((\w+)`)
	restLokiWrapRe    = regexp.MustCompile(`\bwrap\(`)
	restDynamicPathes = map[string]string{
		"POST <base>/query":       "/api/v1/loki/query",
		"POST <base>/query_range": "/api/v1/loki/query_range",
	}
)

type restRoute struct {
	key, path, ability string
}

func parenBalance(s string) int {
	return strings.Count(s, "(") - strings.Count(s, ")")
}

// loadRESTMutatingRoutes reads the router's own registrations so the
// ability each route is gated at comes from source, not a copy.
func loadRESTMutatingRoutes(t *testing.T) []restRoute {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("glob api sources: %v", err)
	}
	var routes []restRoute
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f) //nolint:gosec // package-local glob
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(src), "\n")
		for i, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			m := restRouteDeclRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			rest := m[5]
			// A registration split over lines keeps its wrapper on the next ones.
			for j := i + 1; j < len(lines) && j < i+4 && parenBalance(rest) > 0; j++ {
				rest += " " + strings.TrimSpace(lines[j])
			}
			key := m[1] + " " + m[2]
			path := m[2]
			if m[3] != "" {
				key = m[1] + " <" + m[3] + ">" + m[4]
				path = "/dynamic" + m[4]
				if resolved, ok := restDynamicPathes[key]; ok {
					path = resolved
				}
			}
			ability := ""
			if am := restAbilityRe.FindStringSubmatch(rest); am != nil {
				ability = am[1]
				if ability == "AbilityRead" {
					ability = AbilityRead
				}
			} else if f == "loki.go" && restLokiWrapRe.MatchString(rest) {
				ability = AbilityRead
			}
			routes = append(routes, restRoute{key: key, path: path, ability: ability})
		}
	}
	return routes
}

func restExemptionSet(groups map[string][]string) map[string]string {
	out := map[string]string{}
	for reason, keys := range groups {
		for _, k := range keys {
			out[k] = reason
		}
	}
	return out
}

// Routes that need no audit_log row; each group states why, checked against its handler.
var restExemptions = restExemptionSet(map[string][]string{
	"public webhook or ingest authenticated by its own token, signature or DSN, not an operator mutation": {
		"POST /api/v1/hooks/image-update/{name}/{token}",
		"POST /api/v1/hooks/pipeline/{id}/{token}",
		"POST /api/v1/webhooks/github/{name}",
		"POST /api/v1/webhooks/functions/{name}",
		"POST /api/v1/webhooks/slack/interactions",
		"POST /api/v1/webhooks/discord/interactions",
		"POST /api/v1/git/{repo}/git-receive-pack",
		"POST /api/v1/otlp/v1/traces",
		"POST /api/v1/sentry/api/{project}/envelope/",
		"POST /api/v1/sentry/api/{project}/store/",
		"POST /api/v1/forms/{app}/{name}",
		"PUT /api/v1/function-kv/entry",
		"DELETE /api/v1/function-kv/entry",
		"POST <unsubscribePath>",
	},
	"unauthenticated demo endpoint with no platform state": {
		"POST /api/v1/demo/session",
		"POST /api/v1/demo/cta",
	},
	"pre-auth credential flow; sign-in outcomes land in the account event feed": {
		"POST /api/v1/auth/login",
		"POST /api/v1/auth/register",
		"POST /api/v1/auth/forgot-password",
		"POST /api/v1/auth/reset-password",
		"POST /api/v1/auth/login-approval/poll",
		"POST /api/v1/auth/device/start",
		"POST /api/v1/auth/device/token",
		"POST /api/v1/auth/session-links/consume",
		"POST /api/v1/auth/cli-pairing/redeem",
		"POST /api/v1/auth/2fa/verify",
		"POST /api/v1/auth/passkey-login/begin",
		"POST /api/v1/auth/passkey-login/finish",
		"POST /api/v1/mobile/pair",
		"POST /api/v1/auth/saml/{id}/acs",
	},
	"handler writes its own audit_log row (recordAudit or sign-in audit helper)": {
		"POST /api/v1/auth/login-code/request",
		"POST /api/v1/auth/login-code/redeem",
		"PUT /api/v1/auth/password",
		"POST /api/v1/auth/cli-pairing",
		"POST /api/v1/auth/tokens",
		"DELETE /api/v1/auth/tokens/{id}",
		"POST /api/v1/hooks/deploy/{id}/{token}",
		"POST /api/v1/auth/step-up/passkey/finish",
		"POST /api/v1/auth/step-up",
		"POST /api/v1/auth/sign-in-requests/codes/{id}/reveal",
		"POST /api/v1/auth/login-approvals/{id}/approve",
		"POST /api/v1/auth/login-approvals/{id}/deny",
		"DELETE /api/v1/auth/trusted-devices/{id}",
		"DELETE /api/v1/security/sessions/{id}",
		"POST /api/v1/security/sessions/revoke-others",
		"PUT /api/v1/security/account",
		"DELETE /api/v1/mobile/devices/{id}",

		"POST /api/v1/ci/oidc-exchange",
		"POST /api/v1/auth/sign-in-alert/disown",
		"POST /api/v1/scim/v2/Users",
		"PUT /api/v1/scim/v2/Users/{id}",
		"PATCH /api/v1/scim/v2/Users/{id}",
		"DELETE /api/v1/scim/v2/Users/{id}",
		"POST /api/v1/scim/v2/Groups",
		"PUT /api/v1/scim/v2/Groups/{id}",
		"PATCH /api/v1/scim/v2/Groups/{id}",
		"DELETE /api/v1/scim/v2/Groups/{id}",
	},
	"read-gated preview, lint, plan or probe that persists nothing; the ability gate is the only control": {
		"POST /api/v1/apply/plan",
		"POST /api/v1/apps/{name}/budget/dry-run",
		"POST /api/v1/apps/{name}/domains/{domain}/doctor",
		"POST /api/v1/apps/{name}/domains/{domain}/policies/preview",
		"POST /api/v1/apps/{name}/exposure/check",
		"POST /api/v1/apps/{name}/exposure/preview",
		"POST /api/v1/apps/{name}/git-source/affected",
		"POST /api/v1/apps/{name}/health/discover",
		"POST /api/v1/apps/{name}/log-pipeline/preview",
		"POST /api/v1/apps/{name}/preflight",
		"POST /api/v1/apps/{name}/static-routes/lint",
		"POST /api/v1/apps/{name}/static-routes/test",
		"POST /api/v1/apps/{name}/validate-spec",
		"POST /api/v1/costs/budgets/{id}/dry-run",
		"POST /api/v1/deploy-policies/check",
		"POST /api/v1/docker/lint",
		"POST /api/v1/domains/{domain}/diagnose",
		"POST /api/v1/fixes/{id}/preview",
		"POST /api/v1/iam/policies/validate",
		"POST /api/v1/iam/policy-templates/{id}/render",
		"POST /api/v1/iam/preview",
		"POST /api/v1/iam/resources/match",
		"POST /api/v1/maintenance/preview",
		"POST /api/v1/models/fit",
		"POST /api/v1/models/preflight",
		"POST /api/v1/network-shares/{id}/test",
		"POST /api/v1/pipelines/filters",
		"POST /api/v1/pipelines/validate",
		"POST /api/v1/project-templates/{id}/render",
		"POST /api/v1/prometheus/read",
		"POST /api/v1/stacks/{name}/plan",
		"POST /api/v1/system/doctor/checks/{id}/run",
		"POST /api/v1/auth/step-up/passkey/begin",
	},
	"audited inside the handler or by accountEventMiddleware (account.* rows), which this route scan cannot see": {
		"PATCH /api/v1/auth/profile",
		"POST /api/v1/auth/sessions/revoke-others",
		"POST /api/v1/auth/2fa/setup",
		"POST /api/v1/auth/2fa/confirm",
		"POST /api/v1/auth/2fa/disable",
		"POST /api/v1/auth/2fa/recovery-codes/regenerate",
		"POST /api/v1/auth/passkeys/register/begin",
		"POST /api/v1/auth/passkeys/register/finish",
		"DELETE /api/v1/auth/passkeys/{id}",
		"PATCH /api/v1/auth/passkeys/{id}",
		"POST /api/v1/invites/accept",
		"POST /api/v1/team-invites/accept",
		"DELETE /api/v1/auth/account",
		"DELETE /api/v1/auth/link/{provider}",
		"POST /api/v1/auth/email-change",
		"POST /api/v1/auth/email-change/confirm",
		"POST /api/v1/auth/export",
		"PUT /api/v1/auth/preferences",
		"DELETE /api/v1/teams/{id}/members/me",
	},
	"audited inside the handler by its own lifecycle row (device login, CI exchange)": {
		"POST /api/v1/auth/device/{user_code}/approve",
		"POST /api/v1/auth/device/{user_code}/deny",
		"POST /api/v1/ci/stack-oidc-exchange",
	},
	"per-user UI state or notification preference, no shared platform state": {
		"POST /api/v1/attention/dismiss",
		"POST /api/v1/settings/push-subscriptions",
		"DELETE /api/v1/settings/push-subscriptions/{id}",
		"POST /api/v1/auth/logout",
	},
})

// Real mutating routes with no audit_log row today. Do not add here to silence a new route.
var restKnownAuditGaps = restExemptionSet(map[string][]string{})

func TestRESTMutatingRoutesAreAudited(t *testing.T) {
	routes := loadRESTMutatingRoutes(t)
	var unaudited, stale []string
	seen := map[string]bool{}
	for _, r := range routes {
		seen[r.key] = true
		if r.ability != "" && RouteAudited(strings.Fields(r.key)[0], r.path, r.ability) {
			continue
		}
		if _, ok := restExemptions[r.key]; ok {
			continue
		}
		if _, ok := restKnownAuditGaps[r.key]; ok {
			continue
		}
		unaudited = append(unaudited, r.key)
	}
	for _, m := range []map[string]string{restExemptions, restKnownAuditGaps} {
		for k := range m {
			if !seen[k] {
				stale = append(stale, k)
			}
		}
	}
	sort.Strings(unaudited)
	sort.Strings(stale)
	if len(unaudited) > 0 {
		t.Errorf("mutating routes that are not audited and not exempt:\n  %s\nGate each with rt.requireAbility* at a non-read ability, call recordAudit in the handler, "+
			"or add the exact key to restExemptions in rest_route_audit_test.go with a reason (then bump pinnedExemptions).",
			strings.Join(unaudited, "\n  "))
	}
	if len(stale) > 0 {
		t.Errorf("exemption entries match no route (remove them and lower the pinned counts):\n  %s", strings.Join(stale, "\n  "))
	}
	for k, r := range restExemptions {
		if strings.TrimSpace(r) == "" {
			t.Errorf("exemption %q has no reason", k)
		}
		if _, dup := restKnownAuditGaps[k]; dup {
			t.Errorf("%q is in both exemptions and known gaps", k)
		}
	}
	if len(routes) != pinnedNonGETRoutes {
		t.Errorf("checked %d non-GET routes, pinned %d: a mutating route was added or removed; confirm it is audited, then update pinnedNonGETRoutes", len(routes), pinnedNonGETRoutes)
	}
	if len(restExemptions) != pinnedExemptions {
		t.Errorf("%d exemptions, pinned %d: update pinnedExemptions after reviewing the change", len(restExemptions), pinnedExemptions)
	}
	if len(restKnownAuditGaps) != pinnedKnownAuditGaps {
		t.Errorf("%d known audit gaps, pinned %d: fixing a gap lowers this, a new one must not be added silently", len(restKnownAuditGaps), pinnedKnownAuditGaps)
	}
}
