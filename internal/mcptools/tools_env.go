package mcptools

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/glincker/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const envDeployHint = "Call deploy_app (or restart_app) for the running container to pick this up."

type setAppEnvInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Key    string `json:"key" jsonschema:"the environment variable name"`
	Value  string `json:"value" jsonschema:"the value to store; never returned or logged"`
	Secret bool   `json:"secret,omitempty" jsonschema:"store as a write-only encrypted secret, not a plain env var"`
	Force  bool   `json:"force,omitempty" jsonschema:"secret only: overwrite even if the key is locked"`
	// ExpectedVersion is the env_version from get_app_env; a newer env is a conflict, nothing is changed.
	ExpectedVersion *int `json:"expected_version,omitempty" jsonschema:"env_version read with get_app_env; if the env changed since, the call fails and changes nothing"`
}

type unsetAppEnvInput struct {
	Name   string `json:"name" jsonschema:"the app's name"`
	Key    string `json:"key" jsonschema:"the environment variable name to remove"`
	Secret bool   `json:"secret,omitempty" jsonschema:"remove a secret (and undeclare it) instead of a plain env var"`
	Force  bool   `json:"force,omitempty" jsonschema:"secret only: remove even if the key is locked"`
	// ExpectedVersion is the env_version from get_app_env; a newer env is a conflict, nothing is changed.
	ExpectedVersion *int `json:"expected_version,omitempty" jsonschema:"env_version read with get_app_env; if the env changed since, the call fails and changes nothing"`
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
		Description: "Set one env var on an app. secret true encrypts it and makes it write-only. Applies on the next deploy_app or restart_app.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in setAppEnvInput) (*mcp.CallToolResult, EnvChangeResult, error) {
		res, err := setAppEnv(ctx, client, in)
		if err != nil {
			return nil, EnvChangeResult{}, err
		}
		return nil, res, nil
	})

	addTool(server, &mcp.Tool{
		Name:        "unset_app_env",
		Description: "Remove one env var (plain or secret) from an app. Desired state only: the container keeps it until deploy_app or restart_app.",
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
	req := apiclient.EnvApplyRequest{ExpectedVersion: in.ExpectedVersion}
	if in.Secret {
		req.SecretSet = map[string]string{in.Key: in.Value}
		req.OverwriteLocked = in.Force
	} else {
		req.Set = map[string]string{in.Key: in.Value}
	}
	if _, err := client.ApplyAppEnv(ctx, in.Name, req); err != nil {
		return EnvChangeResult{}, envApplyError("update", in.Key, in.Name, err)
	}
	return res, nil
}

func unsetAppEnv(ctx context.Context, client *apiclient.Client, in unsetAppEnvInput) (EnvChangeResult, error) {
	if err := validateEnvTarget(in.Name, in.Key); err != nil {
		return EnvChangeResult{}, err
	}
	res := EnvChangeResult{Name: in.Name, Key: in.Key, Secret: in.Secret, RedeployNeeded: true, Hint: envDeployHint}
	req := apiclient.EnvApplyRequest{ExpectedVersion: in.ExpectedVersion}
	if in.Secret {
		req.SecretDelete = []string{in.Key}
		req.OverwriteLocked = in.Force
	} else {
		req.Unset = []string{in.Key}
	}
	saved, err := client.ApplyAppEnv(ctx, in.Name, req)
	if err != nil {
		return EnvChangeResult{}, envApplyError("update", in.Key, in.Name, err)
	}
	if !in.Secret && !saved.Changed {
		return EnvChangeResult{}, fmt.Errorf("app %q has no plain env var %q (pass secret true to remove a secret)", in.Name, in.Key)
	}
	return res, nil
}

// envApplyError names the recovery step for a stale expected_version; messages never carry a value.
func envApplyError(verb, key, app string, err error) error {
	var conflict *apiclient.EnvVersionConflictError
	if errors.As(err, &conflict) {
		return fmt.Errorf("env of app %q changed since expected_version, nothing was changed: call get_app_env for the current env_version and retry: %w", app, err)
	}
	return fmt.Errorf("%s env %q on app %q: %w", verb, key, app, err)
}
