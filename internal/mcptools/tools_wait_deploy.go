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

	deployOutcomeFailed = "failed"
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
	Diagnosis *compactDiagnosis               `json:"diagnosis,omitempty"`
	Deploy    apiclient.DeployAttemptResource `json:"deploy"`
}

// waitDeploy blocks up to the env-bounded window (or maxSeconds if smaller)
// and returns the settled or still-running deploy, with a diagnosis on failure.
func waitDeploy(ctx context.Context, client *apiclient.Client, name, deployID string, maxSeconds int) (waitForDeployOutput, error) {
	limit := envSeconds(EnvWaitWindow, defaultWaitWindow)
	if maxSeconds > 0 && time.Duration(maxSeconds)*time.Second < limit {
		limit = time.Duration(maxSeconds) * time.Second
	}
	res, err := client.WaitForDeploy(ctx, name, deployID, apiclient.WaitOptions{Interval: envSeconds(EnvWaitInterval, defaultWaitInterval), Window: limit})
	if err != nil {
		return waitForDeployOutput{}, err
	}
	status := res.Deploy.Outcome
	if !res.Done {
		status = apiclient.DeployOutcomeInProgress
	}
	out := waitForDeployOutput{
		Status: status, PollAgain: !res.Done, DeployID: res.Deploy.ID, App: res.Deploy.ServiceName,
		Failure: res.Deploy.Failure, Deploy: res.Deploy,
	}
	if status == deployOutcomeFailed {
		out.Diagnosis = buildDiagnosis(ctx, client, name, res.Deploy.ID, res.Deploy.Failure)
	}
	return out, nil
}

func registerWaitDeployTool(server *mcp.Server, client *apiclient.Client) {
	addTool(server, &mcp.Tool{
		Name:        "wait_for_deploy",
		Description: "Wait for a deploy to finish: healthy, failed, canceled, superseded or blocked (with the failure). Blocks about 55 seconds; returns in_progress if still running, call again.",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in waitForDeployInput) (*mcp.CallToolResult, waitForDeployOutput, error) {
		out, err := waitDeploy(ctx, client, in.Name, in.DeployID, in.MaxWaitSeconds)
		if err != nil {
			return nil, waitForDeployOutput{}, fmt.Errorf("wait for deploy of app %q: %w", in.Name, err)
		}
		return nil, out, nil
	})
}
