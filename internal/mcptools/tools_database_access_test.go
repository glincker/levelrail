package mcptools

import (
	"regexp"
	"strings"
	"testing"
)

// MCP must never create database users, issue credentials or change network
// rules: those hand out secrets or change exposure.
func TestDatabaseAccessToolsAreReadOnly(t *testing.T) {
	changesAccess := regexp.MustCompile(`(?i)(create|issue|rotate|grant|revoke|delete|drop|allow|deny|apply|set|disable|enable|make).*(database|db).*(user|credential|access|rule|network|tls|scope|private)|(user|credential|access|rule|network|tls|scope).*(create|issue|rotate|grant|revoke|allow|deny|apply)`)
	for name, meta := range toolTable {
		if changesAccess.MatchString(name) && strings.Contains(name, "database") {
			t.Errorf("tool %q looks like it can change database access or network rules, which MCP must never do", name)
		}
		if strings.Contains(name, "database_user") || strings.Contains(name, "database_access") || strings.Contains(name, "database_reachability") {
			if meta.Class != ClassRead {
				t.Errorf("tool %q must be read-only, got %s", name, meta.Class)
			}
		}
	}
	for _, want := range []string{"list_database_users", "get_database_access", "get_database_reachability"} {
		if _, ok := toolTable[want]; !ok {
			t.Errorf("tool %q is not registered in the classification table", want)
		}
	}
}

func TestReachabilitySummaryCarriesNoSecrets(t *testing.T) {
	out := summarizeReachability(sampleNetwork())
	if len(out.AppsOutOfScope) != 1 || out.AppsOutOfScope[0] != "worker" || len(out.Apps) != 1 {
		t.Fatalf("apps split wrong: %+v", out)
	}
	if len(out.Drift) != 1 || !strings.Contains(out.Drift[0], "203.0.113.9") {
		t.Fatalf("drift = %v", out.Drift)
	}
}
