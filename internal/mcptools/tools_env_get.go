package mcptools

import (
	"context"
	"fmt"
	"slices"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AppEnvView is an app's visible plain env vars, plus the names, never
// values, of its secrets and of its sensitive plain vars.
type AppEnvView struct {
	Name          string            `json:"name"`
	Env           map[string]string `json:"env"`
	SensitiveKeys []string          `json:"sensitive_keys"`
	SecretKeys    []string          `json:"secret_keys"`
}

func registerGetEnvTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_app_env",
		Description: "Get an app's plain env vars and the names (never values) of its secrets and sensitive vars. Use before set_app_env or unset_app_env.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, AppEnvView, error) {
		app, err := client.GetApp(ctx, in.Name)
		if err != nil {
			return nil, AppEnvView{}, fmt.Errorf("get env for app %q: %w", in.Name, err)
		}
		stored, err := client.ListSecrets(ctx, in.Name)
		if err != nil {
			return nil, AppEnvView{}, fmt.Errorf("list secrets for app %q: %w", in.Name, err)
		}
		keys := slices.Clone(app.SecretEnv)
		for _, s := range stored {
			if !slices.Contains(keys, s.Key) {
				keys = append(keys, s.Key)
			}
		}
		slices.Sort(keys)
		env := map[string]string{}
		sensitive := []string{}
		for k, v := range app.Env {
			if v == apiclient.MaskedEnvValue {
				sensitive = append(sensitive, k)
				continue
			}
			env[k] = v
		}
		slices.Sort(sensitive)
		if keys == nil {
			keys = []string{}
		}
		return nil, AppEnvView{Name: in.Name, Env: env, SensitiveKeys: sensitive, SecretKeys: keys}, nil
	})
}
