package apiclient

import (
	"context"
	"fmt"
	"time"
)

// Deploy outcomes reported by GetDeploy.
const (
	DeployOutcomeInProgress = "in_progress"
	DeployOutcomeHealthy    = "healthy"
)

// WaitOptions bounds one WaitForDeploy call.
type WaitOptions struct {
	// Interval is the poll period.
	Interval time.Duration
	// Window caps how long this call blocks; zero means until ctx ends.
	Window time.Duration
}

// WaitResult is the last state WaitForDeploy observed.
type WaitResult struct {
	Deploy DeployAttemptResource
	// Done is false when the window elapsed before the deploy finished.
	Done bool
}

// WaitForDeploy polls one deploy until it is healthy, failed, canceled,
// superseded or blocked, or the window elapses. An empty or "latest" deployID
// is resolved once to a concrete id, so a newer deploy cannot take over the
// wait.
func (c *Client) WaitForDeploy(ctx context.Context, name, deployID string, opts WaitOptions) (WaitResult, error) {
	if opts.Interval <= 0 {
		opts.Interval = 2 * time.Second
	}
	if deployID == "" {
		deployID = "latest"
	}
	var deadline <-chan time.Time
	if opts.Window > 0 {
		timer := time.NewTimer(opts.Window)
		defer timer.Stop()
		deadline = timer.C
	}
	ticker := time.NewTicker(opts.Interval)
	defer ticker.Stop()

	var last DeployAttemptResource
	for {
		d, err := c.GetDeploy(ctx, name, deployID)
		if err != nil {
			return WaitResult{Deploy: last}, fmt.Errorf("get deploy %q for app %q: %w", deployID, name, err)
		}
		last = d
		deployID = d.ID
		if d.Outcome != "" && d.Outcome != DeployOutcomeInProgress {
			return WaitResult{Deploy: d, Done: true}, nil
		}
		select {
		case <-ctx.Done():
			return WaitResult{Deploy: last}, ctx.Err()
		case <-deadline:
			return WaitResult{Deploy: last}, nil
		case <-ticker.C:
		}
	}
}
