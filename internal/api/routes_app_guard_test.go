package api

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// legacyPlainAppReadRoutes are app-scoped routes that predate resource-scoped
// guards and still use a plain requireAbility. Do not add to it: new
// /apps/{name}/... routes must use requireAbilityForResource with
// appResourceFromPath so an IAM Deny on the app applies.
var legacyPlainAppReadRoutes = map[string]bool{
	"GET /api/v1/apps/{name}/group":                                              true,
	"GET /api/v1/apps/{name}/hook-runs":                                          true,
	"GET /api/v1/apps/{name}/moves":                                              true,
	"GET /api/v1/apps/{name}/moves/{id}":                                         true,
	"GET /api/v1/apps/{name}/deploys":                                            true,
	"GET /api/v1/apps/{name}/auto-rollback":                                      true,
	"GET /api/v1/apps/{name}/terminal":                                           true,
	"GET /api/v1/apps/{name}/exec-access":                                        true,
	"GET /api/v1/apps/{name}/deploy-attempts":                                    true,
	"GET /api/v1/apps/{name}/deploys/compare":                                    true,
	"GET /api/v1/apps/{name}/promote/preview":                                    true,
	"GET /api/v1/apps/{name}/deploys/{deployId}/logs":                            true,
	"GET /api/v1/apps/{name}/deploys/{deployId}/steps":                           true,
	"GET /api/v1/apps/{name}/deploys/{deployId}/logs/download":                   true,
	"GET /api/v1/apps/{name}/diagnose":                                           true,
	"GET /api/v1/apps/{name}/resource-recommendation":                            true,
	"GET /api/v1/apps/{name}/images":                                             true,
	"GET /api/v1/apps/{name}/network":                                            true,
	"GET /api/v1/apps/{name}/secrets":                                            true,
	"GET /api/v1/apps/{name}/git-source":                                         true,
	"GET /api/v1/apps/{name}/webhook-deliveries":                                 true,
	"GET /api/v1/apps/{name}/previews":                                           true,
	"GET /api/v1/apps/{name}/metrics":                                            true,
	"GET /api/v1/apps/{name}/logs":                                               true,
	"GET /api/v1/apps/{name}/logs/stream":                                        true,
	"GET /api/v1/apps/{name}/logs/download":                                      true,
	"GET /api/v1/apps/{name}/alerts":                                             true,
	"GET /api/v1/apps/{name}/scheduled-tasks":                                    true,
	"GET /api/v1/apps/{name}/scheduled-tasks/{id}":                               true,
	"GET /api/v1/apps/{name}/flags":                                              true,
	"GET /api/v1/apps/{name}/flags/{id}":                                         true,
	"GET /api/v1/apps/{name}/tags":                                               true,
	"GET /api/v1/apps/{name}/integrations":                                       true,
	"GET /api/v1/apps/{name}/deploy-notify-targets":                              true,
	"GET /api/v1/apps/{name}/domains/{domain}/check":                             true,
	"GET /api/v1/apps/{name}/domains/{domain}/auth":                              true,
	"GET /api/v1/apps/{name}/domains/{domain}/maintenance":                       true,
	"GET /api/v1/apps/{name}/domains/{domain}/tls-cert":                          true,
	"GET /api/v1/apps/{name}/domains/{domain}/waf":                               true,
	"GET /api/v1/apps/{name}/domains/{domain}/redirect":                          true,
	"GET /api/v1/apps/{name}/domains/{domain}/error-pages":                       true,
	"GET /api/v1/apps/{name}/volumes/{volume}/backups":                           true,
	"GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/download":      true,
	"GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verifications": true,
	"GET /api/v1/apps/{name}/volumes/{volume}/backup-schedule":                   true,
	"GET /api/v1/apps/{name}/volumes/{volume}/restores":                          true,
	"GET /api/v1/apps/{name}/volumes/{volume}/clone-restores":                    true,
	"GET /api/v1/apps/{name}/log-drain":                                          true,
}

func TestAppRoutesUseResourceScopedGuard(t *testing.T) {
	reg := regexp.MustCompile(`mux\.HandleFunc\("([A-Z]+) (/api/v1/apps/\{name\}[^"]*)", rt\.requireAbility\(`)
	files, err := filepath.Glob("routes*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no routes files found: %v", err)
	}
	seen := map[string]bool{}
	for _, f := range files {
		src, err := os.ReadFile(f) //nolint:gosec // globbed routes files in the package dir
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range reg.FindAllStringSubmatch(string(src), -1) {
			key := m[1] + " " + m[2]
			seen[key] = true
			if !legacyPlainAppReadRoutes[key] {
				t.Errorf("%s (%s) uses plain requireAbility; use requireAbilityForResource(..., appResourceFromPath, ...)", key, f)
			}
		}
	}
	for key := range legacyPlainAppReadRoutes {
		if !seen[key] {
			t.Errorf("stale allowlist entry %q: route now guarded or removed, delete it from legacyPlainAppReadRoutes", key)
		}
	}
}
