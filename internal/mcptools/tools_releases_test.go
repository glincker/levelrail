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
	for _, name := range []string{"list_releases", "get_rollback_plan"} {
		if !registered[name] {
			t.Errorf("%s is not registered", name)
		}
		m, ok := toolTable[name]
		if !ok || m.Class != ClassRead || !m.Untrusted() {
			t.Errorf("%s must be a read-class tool marked untrusted (release notes), got %+v", name, m)
		}
	}
}
