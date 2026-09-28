package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// TriggerScheduledDeploy resolves serviceName's git source's latest
// commit on branch (no clone, resolveBranchSHA) and deploys it through
// the same path a webhook-triggered push uses (deployFromGitSourceAs),
// tagged store.DeployAttemptSourceSchedule so it shows up under
// ListDeployments' "schedule" trigger filter. The one difference from a
// real push: branch may not be gs.Branch, a scheduled redeploy can target
// any branch, gs is only consulted for repo_url/build config.
//
// Satisfies internal/scheduledeploy.Trigger; called only from that
// package's Scheduler, never directly from an HTTP handler, so it
// returns a plain error rather than an (int, string) status pair.
func (rt *Router) TriggerScheduledDeploy(ctx context.Context, serviceName, branch string) (string, error) {
	gs, err := rt.gitSources.GetGitSource(ctx, serviceName)
	if err != nil {
		if errors.Is(err, store.ErrGitSourceNotFound) {
			return "", fmt.Errorf("no git source connected for %q", serviceName)
		}
		return "", fmt.Errorf("load git source for %q: %w", serviceName, err)
	}

	token, err := rt.resolveGitSourceDeployToken(ctx, serviceName)
	if err != nil {
		return "", fmt.Errorf("resolve deploy token for %q: %w", serviceName, err)
	}

	sha, err := rt.resolveBranchSHA(ctx, gs.RepoURL, branch, token)
	if err != nil {
		return "", fmt.Errorf("resolve latest commit for %q@%q: %w", serviceName, branch, err)
	}

	targeted := *gs
	targeted.Branch = branch
	status, message := rt.deployFromGitSourceAs(ctx, serviceName, targeted, sha, sha, rt.nextOrder(ctx, serviceName, sha, "", time.Time{}), store.DeployAttemptSourceSchedule)
	message = strings.TrimSpace(message)
	if status >= http.StatusBadRequest {
		if message == "" {
			message = fmt.Sprintf("deploy failed with status %d", status)
		}
		return "", errors.New(message)
	}
	return message, nil
}
