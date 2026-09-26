package api

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// publicRoutes is the complete set of routes intentionally registered
// without requireAbility/requireAuth. Each must either carry no data worth
// protecting or authenticate itself (token, HMAC, or a rate-limited
// credential exchange).
var publicRoutes = map[string]string{ //nolint:gosec // route patterns, not credentials
	"GET /healthz":                               "liveness probe",
	"GET /readyz":                                "readiness probe, reports only component health",
	"GET /api/v1/brand":                          "login screen branding",
	"GET /api/v1/dev-mode":                       "dev-mode banner flag",
	"POST /api/v1/auth/login":                    "credential exchange, rate limited",
	"POST /api/v1/auth/register":                 "first-user setup, gated by setup token",
	"GET /api/v1/auth/setup-status":              "needs-setup boolean",
	"POST /api/v1/auth/2fa/verify":               "second factor exchange, rate limited",
	"POST /api/v1/invites/accept":                "gated by emailed invite token",
	"POST /api/v1/auth/device/start":             "device login start, rate limited",
	"POST /api/v1/auth/device/token":             "device code exchange",
	"GET /api/v1/auth/oauth/providers":           "login screen provider list",
	"GET /api/v1/auth/oauth/{provider}/start":    "OAuth redirect",
	"GET /api/v1/auth/oauth/{provider}/callback": "OAuth callback, state checked",
	"POST /api/v1/auth/forgot-password":          "rate limited, no account enumeration",
	"POST /api/v1/auth/reset-password":           "gated by emailed reset token",
	"POST /api/v1/webhooks/github/{name}":        "gated by per-app HMAC signature",
}

var routeRegistration = regexp.MustCompile(`^mux\.Handle(?:Func)?\("([^"]+)",\s*(.*)$`)

func TestEveryRouteIsGuardedOrExplicitlyPublic(t *testing.T) {
	guards := []string{"rt.requireAbility(", "rt.requireAbilityForResource(", "rt.requireAuth(", "rt.requireAbilityDecided("}
	seenPublic := map[string]bool{}
	registrations := 0
	for _, file := range []string{"routes.go", "routes_platform.go"} {
		src, err := os.ReadFile(file) //nolint:gosec // fixed source file names from the literal above
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			m := routeRegistration.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil {
				continue
			}
			registrations++
			pattern, rest := m[1], m[2]
			guarded := false
			for _, g := range guards {
				if strings.HasPrefix(rest, g) {
					guarded = true
				}
			}
			_, isPublic := publicRoutes[pattern]
			switch {
			case guarded && isPublic:
				t.Errorf("%s:%d %q is guarded but listed in publicRoutes; remove it there", file, i+1, pattern)
			case !guarded && !isPublic:
				t.Errorf("%s:%d %q is registered without an ability guard and is not in publicRoutes", file, i+1, pattern)
			case !guarded:
				seenPublic[pattern] = true
			}
		}
	}
	if registrations < 100 {
		t.Fatalf("only %d route registrations found, the scanner is out of date", registrations)
	}
	for p := range publicRoutes {
		if !seenPublic[p] {
			t.Errorf("publicRoutes lists %q but no such unguarded registration exists; remove the stale entry", p)
		}
	}
}
