package mcptools

import (
	"context"
	"fmt"
	"sort"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type setAppEnvInput struct {
	Name  string            `json:"name" jsonschema:"the app's name"`
	Set   map[string]string `json:"set,omitempty" jsonschema:"plain env vars to add or change, KEY to value"`
	Unset []string          `json:"unset,omitempty" jsonschema:"env var names to remove"`
	Apply bool              `json:"apply,omitempty" jsonschema:"restart the app now so the change reaches the running container"`
}

type setAppSecretInput struct {
	Name  string `json:"name" jsonschema:"the app's name"`
	Key   string `json:"key" jsonschema:"secret env var name"`
	Value string `json:"value" jsonschema:"the secret value; stored encrypted and never returned"`
}

type appEnvView struct {
	Name       string            `json:"name"`
	Env        map[string]string `json:"env"`
	SecretKeys []string          `json:"secret_keys"`
}

type setAppEnvResult struct {
	Name      string   `json:"name"`
	Changed   []string `json:"changed"`
	Removed   []string `json:"removed"`
	Skipped   []string `json:"skipped_secret_keys,omitempty"`
	Restarted bool     `json:"restarted"`
}

func registerAppEnvTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "get_app_env",
		Description: "Get an app's plain env vars and the names (never values) of its secret env vars. Use before set_app_env or set_app_secret.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in appNameInput) (*mcp.CallToolResult, any, error) {
		app, err := client.GetApp(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("get env for app %q: %w", in.Name, err)
		}
		keys, err := appSecretKeyNames(ctx, client, in.Name, app.SecretEnv)
		if err != nil {
			return nil, nil, err
		}
		env := app.Env
		if env == nil {
			env = map[string]string{}
		}
		return nil, appEnvView{Name: in.Name, Env: env, SecretKeys: keys}, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "set_app_env",
		Description: "Add, change or remove plain env vars on an app; keys that are secrets are skipped, use set_app_secret for those. Takes effect on the next deploy or restart, or now with apply.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setAppEnvInput) (*mcp.CallToolResult, any, error) {
		if len(in.Set) == 0 && len(in.Unset) == 0 {
			return nil, nil, fmt.Errorf("set env for app %q: pass set or unset", in.Name)
		}
		app, err := client.GetApp(ctx, in.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("get app %q: %w", in.Name, err)
		}
		secrets, err := appSecretKeyNames(ctx, client, in.Name, app.SecretEnv)
		if err != nil {
			return nil, nil, err
		}
		isSecret := map[string]bool{}
		for _, k := range secrets {
			isSecret[k] = true
		}
		res := setAppEnvResult{Name: in.Name, Changed: []string{}, Removed: []string{}}
		env := map[string]string{}
		for k, v := range app.Env {
			env[k] = v
		}
		for k, v := range in.Set {
			if isSecret[k] {
				res.Skipped = append(res.Skipped, k)
				continue
			}
			env[k] = v
			res.Changed = append(res.Changed, k)
		}
		for _, k := range in.Unset {
			if isSecret[k] {
				res.Skipped = append(res.Skipped, k)
				continue
			}
			if _, ok := env[k]; ok {
				delete(env, k)
				res.Removed = append(res.Removed, k)
			}
		}
		sort.Strings(res.Changed)
		sort.Strings(res.Removed)
		sort.Strings(res.Skipped)
		if len(res.Changed) == 0 && len(res.Removed) == 0 {
			return nil, res, nil
		}
		app.Env = env
		if _, err := client.UpdateApp(ctx, in.Name, app); err != nil {
			return nil, nil, fmt.Errorf("update env for app %q: %w", in.Name, err)
		}
		if in.Apply {
			if _, err := client.RestartApp(ctx, in.Name); err != nil {
				return nil, nil, fmt.Errorf("env saved but restart of app %q failed: %w", in.Name, err)
			}
			res.Restarted = true
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "set_app_secret",
		Description: "Store one secret env var on an app, encrypted; the value is never returned. Fails if the secret is locked. Takes effect on the next deploy or restart.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setAppSecretInput) (*mcp.CallToolResult, any, error) {
		if in.Key == "" || in.Value == "" {
			return nil, nil, fmt.Errorf("set secret for app %q: key and value are required", in.Name)
		}
		if err := client.SetSecret(ctx, in.Name, in.Key, in.Value, false); err != nil {
			return nil, nil, fmt.Errorf("set secret %q for app %q: %w", in.Key, in.Name, err)
		}
		return nil, map[string]any{"name": in.Name, "key": in.Key, "stored": true}, nil
	})
}

func appSecretKeyNames(ctx context.Context, client *apiclient.Client, name string, declared []string) ([]string, error) {
	stored, err := client.ListSecrets(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("list secrets for app %q: %w", name, err)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, k := range declared {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	for _, s := range stored {
		if !seen[s.Key] {
			seen[s.Key] = true
			out = append(out, s.Key)
		}
	}
	sort.Strings(out)
	return out, nil
}
