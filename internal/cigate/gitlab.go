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
	// Ref scopes the read to one branch or tag; empty reads every ref on the SHA.
	Ref string
}

// Checks returns the latest status of every job on sha.
func (g *GitLab) Checks(ctx context.Context, sha string) ([]Check, error) {
	if g.Client == nil {
		return nil, errors.New("cigate: no GitLab client configured")
	}
	list, err := g.Client.ListCommitStatuses(ctx, g.InstanceURL, g.Token, g.ProjectPath, sha, g.Ref)
	if err != nil {
		if ra, ok := gitprovider.IsRateLimited(err); ok {
			return nil, &RateLimitError{RetryAfter: parseRetryAfter(ra)}
		}
		if errors.Is(err, gitlabapp.ErrTooManyStatuses) {
			return nil, &TerminalError{Reason: "commit has too many CI statuses to read"}
		}
		return nil, fmt.Errorf("cigate: read GitLab checks: %w", err)
	}
	// The same job name can come from several pipelines (or a forged external
	// status); the worst one decides.
	worst := map[string]Check{}
	order := make([]string, 0, len(list))
	for _, s := range list {
		c := gitlabCheck(s)
		prev, ok := worst[c.Name]
		if !ok {
			order = append(order, c.Name)
		}
		if !ok || severity(c) > severity(prev) {
			worst[c.Name] = c
		}
	}
	out := make([]Check, 0, len(order))
	for _, n := range order {
		out = append(out, worst[n])
	}
	return out, nil
}

// gitlabCheck fails closed: an unknown status stays Pending, never Passed.
// Skipped jobs and allowed failures pass softly, so a required name never
// counts them (a skipped job usually means an earlier stage failed).
func gitlabCheck(s gitlabapp.CommitStatus) Check {
	c := Check{Name: s.Name, State: Pending}
	switch s.Status {
	case "success":
		c.State = Passed
	case "skipped":
		c.State = Skipped
	case "failed":
		if s.AllowFailure {
			c.State = Skipped
		} else {
			c.State = Failed
		}
	case "canceled", "canceling":
		c.State = Failed
	case "manual":
		// A manual job nobody has to run does not block the pipeline.
		if s.AllowFailure {
			c.State = Skipped
		}
	}
	return c
}

func parseRetryAfter(v string) time.Duration {
	if s, err := strconv.Atoi(v); err == nil && s > 0 {
		return time.Duration(s) * time.Second
	}
	return rateLimitMinDelay
}
