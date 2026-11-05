package cigate

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
)

// GitLab reads job and commit statuses from the GitLab REST API. Every read
// goes to the API for the pushed SHA; a webhook body is never consulted.
type GitLab struct {
	// Client should come from gitlabapp.NewClient so the SSRF guard and timeout apply.
	Client      *gitlabapp.Client
	InstanceURL string
	Token       string
	// ProjectPath is "namespace/project" or "namespace/subgroup/project".
	ProjectPath string
}

// Checks returns the latest status of every job on sha.
func (g *GitLab) Checks(ctx context.Context, sha string) ([]Check, error) {
	if g.Client == nil {
		return nil, errors.New("cigate: no GitLab client configured")
	}
	list, err := g.Client.ListCommitStatuses(ctx, g.InstanceURL, g.Token, g.ProjectPath, sha)
	if err != nil {
		if ra, ok := gitprovider.IsRateLimited(err); ok {
			return nil, &RateLimitError{RetryAfter: parseRetryAfter(ra)}
		}
		return nil, fmt.Errorf("cigate: read GitLab checks: %w", err)
	}
	out := make([]Check, 0, len(list))
	for _, s := range list {
		out = append(out, Check{Name: s.Name, State: gitlabState(s)})
	}
	return out, nil
}

// gitlabState fails closed: an unknown status stays Pending, never Passed.
func gitlabState(s gitlabapp.CommitStatus) State {
	switch s.Status {
	case "success", "skipped":
		return Passed
	case "failed":
		if s.AllowFailure {
			return Passed
		}
		return Failed
	case "canceled", "canceling":
		return Failed
	case "manual":
		// A manual job nobody has to run does not block the pipeline.
		if s.AllowFailure {
			return Passed
		}
		return Pending
	}
	return Pending
}

func parseRetryAfter(v string) time.Duration {
	if s, err := strconv.Atoi(v); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	return rateLimitMinDelay
}
