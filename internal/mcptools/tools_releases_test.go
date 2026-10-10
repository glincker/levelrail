package mcptools

import (
	"strings"
	"testing"
)

// appRollbackTools are per-app deploy rollbacks, unrelated to the control
// plane's own release.
var appRollbackTools = map[string]bool{"rollback_app": true, "rollback_app_to_deploy": true}

func TestNoToolAppliesAControlPlaneUpgradeOrRollback(t *testing.T) {
	registered := map[string]bool{}
	for _, tool := range listAll(t, Options{Mode: ModeFull}) {
		registered[tool.Name] = true
		name := strings.ToLower(tool.Name)
		isRead := strings.HasPrefix(name, "get_") || strings.HasPrefix(name, "list_")
		if (strings.Contains(name, "rollback") || strings.Contains(name, "upgrade")) && !isRead && !appRollbackTools[name] {
			t.Errorf("tool %q looks like it applies a control plane upgrade or rollback", tool.Name)
		}
	}
	for _, name := range []string{"list_releases", "get_rollback_plan", "list_upgrade_history"} {
		if !registered[name] {
			t.Errorf("%s is not registered", name)
		}
		m, ok := toolTable[name]
		if !ok || m.Class != ClassRead || !m.Untrusted() {
			t.Errorf("%s must be a read-class tool marked untrusted (release notes), got %+v", name, m)
		}
	}
}

func TestNoToolCanAcknowledgeOrModifyUpgradeHistory(t *testing.T) {
	for _, tool := range listAll(t, Options{Mode: ModeFull}) {
		name := strings.ToLower(tool.Name)
		if strings.Contains(name, "upgrade_history") && name != "list_upgrade_history" {
			t.Errorf("tool %q touches upgrade history; only list_upgrade_history may exist", tool.Name)
		}
		if strings.Contains(name, "ack") && strings.Contains(name, "upgrade") {
			t.Errorf("tool %q can acknowledge upgrades", tool.Name)
		}
	}
}
