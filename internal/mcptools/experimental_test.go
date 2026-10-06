package mcptools

import (
	"os"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMain enables every experimental feature so the rest of the package
// sees the full tool surface; the gate test narrows it per case.
func TestMain(m *testing.M) {
	experimental.Set(experimental.All()...)
	os.Exit(m.Run())
}

func TestExperimentalToolsHiddenByDefault(t *testing.T) {
	defer experimental.Set(experimental.All()...)
	tests := []struct {
		feature experimental.Feature
		tools   []string
	}{
		{experimental.AIModels, []string{"list_models", "deploy_model", "list_gpu_nodes"}},
		{experimental.LoadBalancer, []string{"list_load_balancers", "set_app_load_balancer"}},
		{experimental.IaC, []string{"plan_apply", "apply_resources"}},
		{experimental.CloudflareTunnel, []string{"get_cloudflare_tunnel_status"}},
		{experimental.AccessRoles, []string{"list_roles"}},
		{experimental.GlobalEnvironments, []string{"list_global_environments", "move_app_environment"}},
	}
	for _, tc := range tests {
		t.Run(string(tc.feature), func(t *testing.T) {
			experimental.Set()
			off := names(listAll(t, Options{Mode: ModeFull}))
			experimental.Set(tc.feature)
			on := names(listAll(t, Options{Mode: ModeFull}))
			for _, n := range tc.tools {
				if off[n] {
					t.Errorf("%s registered while %s is off", n, tc.feature)
				}
				if !on[n] {
					t.Errorf("%s missing while %s is on", n, tc.feature)
				}
			}
		})
	}
}

func names(in []*mcp.Tool) map[string]bool {
	out := map[string]bool{}
	for _, x := range in {
		out[x.Name] = true
	}
	return out
}
