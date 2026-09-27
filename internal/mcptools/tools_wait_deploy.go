package mcptools

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Env vars bounding one wait_for_deploy call, in seconds.
const (
	EnvWaitWindow   = "APP_MCP_WAIT_WINDOW_SECONDS"
	EnvWaitInterval = "APP_MCP_WAIT_POLL_SECONDS"

	defaultWaitWindow   = 55 * time.Second
	defaultWaitInterval = 2 * time.Second
)

func envSeconds(key string, def time.Duration) time.Duration {
	if n, err := strconv.Atoi(os.Getenv(key)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return def
}

type waitForDeployInput struct {
	Name           string `json:"name" jsonschema:"the app's name"`
	DeployID       string `json:"deploy_id,omitempty" jsonschema:"the deploy attempt id, or latest (default); latest is resolved once so a newer deploy cannot take over"`
	MaxWaitSeconds int    `json:"max_wait_seconds,omitempty" jsonschema:"block at most this long, capped by the server limit (default 55)"`
}

type waitForDeployOutput struct {
	Status    string                          `json:"status" jsonschema:"in_progress, healthy, failed, canceled, superseded or blocked"`
	PollAgain bool                            `json:"poll_again" jsonschema:"true when the deploy is still running: call again with the same deploy_id"`
	DeployID  string                          `json:"deploy_id"`
	App       string                          `json:"app"`
	Failure   *apiclient.DeployFailure        `json:"failure,omitempty"`
	Deploy    apiclient.DeployAttemptResource `json:"deploy"`
}

func registerWaitDeployTool(server *mcp.Server, client *apiclient.Client) {
	window := envSeconds(EnvWaitWindow, defaultWaitWindow)
	interval := envSeconds(EnvWaitInterval, defaultWaitInterval)

	addTool(server, &mcp.Tool{
		Name:        "wait_for_deploy",
		Description: "Block until a deploy is healthy, failed, canceled, superseded or blocked, then return its status and, on failure, the structured failure object. One call blocks at most about 55 seconds: when the deploy is still running the result is status in_progress with poll_again true and the concrete deploy_id, so call again with that deploy_id. Read-only.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in waitForDeployInput) (*mcp.CallToolResult, waitForDeployOutput, error) {
		limit := window
		if in.MaxWaitSeconds > 0 && time.Duration(in.MaxWaitSeconds)*time.Second < limit {
			limit = time.Duration(in.MaxWaitSeconds) * time.Second
		}
		res, err := client.WaitForDeploy(ctx, in.Name, in.DeployID, apiclient.WaitOptions{Interval: interval, Window: limit})
		if err != nil {
			return nil, waitForDeployOutput{}, fmt.Errorf("wait for deploy of app %q: %w", in.Name, err)
		}
		status := res.Deploy.Outcome
		if !res.Done {
			status = apiclient.DeployOutcomeInProgress
		}
		return nil, waitForDeployOutput{
			Status: status, PollAgain: !res.Done, DeployID: res.Deploy.ID, App: res.Deploy.ServiceName,
			Failure: res.Deploy.Failure, Deploy: res.Deploy,
		}, nil
	})
}
