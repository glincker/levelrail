package mcptools

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestInspectToolsetsAreReadOnlyAndRootTierIsFiltered(t *testing.T) {
	groups := map[string]bool{"security": true, "migrate": true, "identity": true}
	seen := map[string]int{}
	for name, m := range toolTable {
		if !groups[m.Group] {
			continue
		}
		seen[m.Group]++
		if m.Class != ClassRead {
			t.Errorf("%s in %s must be read, got %s", name, m.Group, m.Class)
		}
		for _, banned := range []string{"start", "run_", "rollback", "cutover_start", "takeover", "connect_", "apply"} {
			if strings.Contains(name, banned) {
				t.Errorf("%s looks like an operation the migrate, security and identity toolsets must never offer", name)
			}
		}
		_, root := rootTierTools[name]
		if m.Group == "identity" && !root {
			t.Errorf("%s is an identity tool and must be root tier", name)
		}
		if root && abilityAllows([]string{abilityRead}, name, m) {
			t.Errorf("%s needs root but a read token is allowed", name)
		}
		if !root && !abilityAllows([]string{abilityRead}, name, m) {
			t.Errorf("%s should be callable with read", name)
		}
	}
	for g := range groups {
		if seen[g] == 0 {
			t.Errorf("toolset %s has no tools", g)
		}
	}
}

func TestInspectToolsHideSecrets(t *testing.T) {
	session := newTestSession(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/sso/settings":
			_, _ = w.Write([]byte(`{"enforce_sso":true,"scim_base_url":"https://x/scim","sp_certificate":"PEM"}`))
		case "/api/v1/sso/scim/tokens":
			_, _ = w.Write([]byte(`[{"id":"t1","name":"okta","prefix":"scim_ab","revoked_at":"2026-01-01T00:00:00Z"}]`))
		case "/api/v1/sso/saml/connections":
			_, _ = w.Write([]byte(`[{"id":"c1","name":"Okta","enabled":true,"idp_certificates":"SECRETPEM","certificates":[{"subject":"a","not_after":"2027-01-01"},{"subject":"b","not_after":"2026-06-01"}]}]`))
		case "/api/v1/settings/beta-features":
			_, _ = w.Write([]byte(`[{"id":"saml-scim","enabled":true,"maturity":"beta"}]`))
		case "/api/v1/export/destinations":
			_, _ = w.Write([]byte(`[{"id":"d1","name":"siem","kind":"http","endpoint":"https://u:tok@h/x","enabled":true,"status":{"last_success_at":"2026-01-01","last_error_at":"2026-01-02","last_error":"boom","circuit_open":false}}]`))
		default:
			http.NotFound(w, r)
		}
	})
	call := func(name string) string {
		t.Helper()
		res, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name})
		if err != nil || res.IsError {
			t.Fatalf("%s: %v %+v", name, err, res)
		}
		var sb strings.Builder
		for _, c := range res.Content {
			if tc, ok := c.(*mcp.TextContent); ok {
				sb.WriteString(tc.Text)
			}
		}
		return sb.String()
	}
	if out := call("list_sso_connections"); strings.Contains(out, "SECRETPEM") || !strings.Contains(out, "2026-06-01") || !strings.Contains(out, `"beta"`) {
		t.Errorf("sso connections output wrong: %s", out)
	}
	if out := call("get_scim_status"); strings.Contains(out, "PEM") || !strings.Contains(out, `"revoked":true`) {
		t.Errorf("scim status output wrong: %s", out)
	}
	out := call("list_export_destinations")
	if strings.Contains(out, "tok@") || !strings.Contains(out, `"behind":true`) {
		t.Errorf("export destinations output wrong: %s", out)
	}
}
