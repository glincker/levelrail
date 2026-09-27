package mcptools

import (
	"context"
	"fmt"
	"slices"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AppEnvView is an app's plain env vars and the names, never values, of its secrets.
type AppEnvView struct {
	Name       string            `json:"name"`
	Env        map[string]string `json:"env"`
	SecretKeys []string          `json:"secret_keys"`
}

func registerGetEnvTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_app_env",
		Description: "Get an app's plain env vars and the names (never values) of its secrets. Use before set_app_env or unset_app_env.",
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
		env := app.Env
		if env == nil {
			env = map[string]string{}
		}
		if keys == nil {
			keys = []string{}
		}
		return nil, AppEnvView{Name: in.Name, Env: env, SecretKeys: keys}, nil
	})
}
