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

// TriggerScheduledDeploy deploys serviceName's git source at the latest commit on branch, which may differ from gs.Branch.
// Called only by the scheduler, so it returns a plain error.
func (rt *Router) TriggerScheduledDeploy(ctx context.Context, serviceName, branch string) (string, error) {
	return rt.triggerGitDeploy(ctx, serviceName, branch, store.DeployAttemptSourceSchedule)
}

// triggerGitDeploy deploys the latest commit on branch, recording the attempt under source.
func (rt *Router) triggerGitDeploy(ctx context.Context, serviceName, branch, source string) (string, error) {
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
	status, message := rt.deployFromGitSourceAs(ctx, serviceName, targeted, sha, sha, rt.nextOrder(ctx, serviceName, sha, "", time.Time{}), source)
	message = strings.TrimSpace(message)
	if status >= http.StatusBadRequest {
		if message == "" {
			message = fmt.Sprintf("deploy failed with status %d", status)
		}
		return "", errors.New(message)
	}
	return message, nil
}
