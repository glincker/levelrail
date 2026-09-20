package ai

import "testing"

func TestIsReadOnly(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		want     bool
	}{
		// Real read-only tools from internal/mcptools.
		{"list_apps", "list_apps", true},
		{"get_app", "get_app", true},
		{"get_app_logs", "get_app_logs", true},
		{"get_app_metrics", "get_app_metrics", true},
		{"diagnose_app_failure", "diagnose_app_failure", true},
		{"list_deploy_attempts", "list_deploy_attempts", true},
		{"get_database", "get_database", true},
		{"list_nodes", "list_nodes", true},
		{"get_system_status", "get_system_status", true},
		{"get_onboarding_status", "get_onboarding_status", true},

		// Real mutating tools from internal/mcptools.
		{"deploy_app", "deploy_app", false},
		{"deploy_compose", "deploy_compose", false},
		{"rollback_app", "rollback_app", false},
		{"restart_app", "restart_app", false},
		{"clone_app", "clone_app", false},
		{"prune_system", "prune_system", false},
		{"preview_promote_app", "preview_promote_app", false},
		{"promote_app", "promote_app", false},
		{"sweep_stale_preview_environments", "sweep_stale_preview_environments", false},

		// Ambiguous real tools (neither list/get/diagnose): fail-safe
		// default is mutating, requiring confirmation even though they
		// may in fact be side-effect-free (a DNS check, a connectivity
		// test, a comparison). The heuristic is deliberately conservative.
		{"check_domain_dns treated as mutating (fail-safe)", "check_domain_dns", false},
		{"test_backup_target_connection treated as mutating (fail-safe)", "test_backup_target_connection", false},
		{"compare_deploys treated as mutating (fail-safe)", "compare_deploys", false},

		// Edge cases.
		{"empty string", "", false},
		{"no underscore", "restart", false},
		{"list with no suffix", "list_", true},
		{"prefix substring but not list/get/diagnose", "listing_apps", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsReadOnly(tt.toolName); got != tt.want {
				t.Errorf("IsReadOnly(%q) = %v, want %v", tt.toolName, got, tt.want)
			}
		})
	}
}
