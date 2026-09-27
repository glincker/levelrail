package mcptools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const envDeployHint = "Call deploy_app (or restart_app) for the running container to pick this up."

type setAppEnvInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Key    string `json:"key" jsonschema:"the environment variable name"`
	Value  string `json:"value" jsonschema:"the value to store; never returned or logged"`
	Secret bool   `json:"secret,omitempty" jsonschema:"store the value envelope-encrypted as a write-only secret instead of a plain env var"`
	Force  bool   `json:"force,omitempty" jsonschema:"secret only: overwrite even if the key is locked"`
}

type unsetAppEnvInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Key    string `json:"key" jsonschema:"the environment variable name to remove"`
	Secret bool   `json:"secret,omitempty" jsonschema:"remove a secret (and undeclare it) instead of a plain env var"`
	Force  bool   `json:"force,omitempty" jsonschema:"secret only: remove even if the key is locked"`
}

// EnvChangeResult reports which key changed, never any value.
type EnvChangeResult struct {
	Name           string `json:"name"`
	Key            string `json:"key"`
	Secret         bool   `json:"secret"`
	RedeployNeeded bool   `json:"redeploy_needed"`
	Hint           string `json:"hint"`
}

func registerEnvTools(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "set_app_env",
		Description: "Set or change one environment variable on an app. With secret true the value is stored envelope-encrypted and is write-only: it is never returned by any tool. The change is saved to desired state only; the running container keeps its old environment until the app is redeployed or restarted (deploy_app, restart_app). Returns the key and whether a redeploy is needed, never the value.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setAppEnvInput) (*mcp.CallToolResult, EnvChangeResult, error) {
		res, err := setAppEnv(ctx, client, in)
		if err != nil {
			return nil, EnvChangeResult{}, err
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "unset_app_env",
		Description: "Remove one environment variable from an app, plain or secret. The change is saved to desired state only; the running container keeps its old environment until the app is redeployed or restarted (deploy_app, restart_app). Returns the key and whether a redeploy is needed.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in unsetAppEnvInput) (*mcp.CallToolResult, EnvChangeResult, error) {
		res, err := unsetAppEnv(ctx, client, in)
		if err != nil {
			return nil, EnvChangeResult{}, err
		}
		return nil, res, nil
	})
}

func validateEnvTarget(name, key string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(key) == "" {
		return errors.New("name and key are required")
	}
	return nil
}

func setAppEnv(ctx context.Context, client *apiclient.Client, in setAppEnvInput) (EnvChangeResult, error) {
	if err := validateEnvTarget(in.Name, in.Key); err != nil {
		return EnvChangeResult{}, err
	}
	res := EnvChangeResult{Name: in.Name, Key: in.Key, Secret: in.Secret, RedeployNeeded: true, Hint: envDeployHint}
	if in.Secret {
		if err := client.SetSecret(ctx, in.Name, in.Key, in.Value, in.Force); err != nil {
			return EnvChangeResult{}, fmt.Errorf("set secret %q on app %q: %w", in.Key, in.Name, err)
		}
		return res, nil
	}
	app, err := client.GetApp(ctx, in.Name)
	if err != nil {
		return EnvChangeResult{}, fmt.Errorf("get app %q: %w", in.Name, err)
	}
	if app.Env == nil {
		app.Env = map[string]string{}
	}
	app.Env[in.Key] = in.Value
	saved, err := client.UpdateApp(ctx, in.Name, app)
	if err != nil {
		return EnvChangeResult{}, fmt.Errorf("update env %q on app %q: %w", in.Key, in.Name, err)
	}
	res.RedeployNeeded = saved.EnvDirty
	return res, nil
}

func unsetAppEnv(ctx context.Context, client *apiclient.Client, in unsetAppEnvInput) (EnvChangeResult, error) {
	if err := validateEnvTarget(in.Name, in.Key); err != nil {
		return EnvChangeResult{}, err
	}
	res := EnvChangeResult{Name: in.Name, Key: in.Key, Secret: in.Secret, RedeployNeeded: true, Hint: envDeployHint}
	if in.Secret {
		if err := client.DeleteSecret(ctx, in.Name, in.Key, in.Force); err != nil {
			return EnvChangeResult{}, fmt.Errorf("delete secret %q on app %q: %w", in.Key, in.Name, err)
		}
		return res, nil
	}
	app, err := client.GetApp(ctx, in.Name)
	if err != nil {
		return EnvChangeResult{}, fmt.Errorf("get app %q: %w", in.Name, err)
	}
	if _, ok := app.Env[in.Key]; !ok {
		return EnvChangeResult{}, fmt.Errorf("app %q has no plain env var %q (pass secret true to remove a secret)", in.Name, in.Key)
	}
	delete(app.Env, in.Key)
	saved, err := client.UpdateApp(ctx, in.Name, app)
	if err != nil {
		return EnvChangeResult{}, fmt.Errorf("update env %q on app %q: %w", in.Key, in.Name, err)
	}
	res.RedeployNeeded = saved.EnvDirty
	return res, nil
}
