package gitlabapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

const (
	statusPageSize = 100
	// statusMaxPages bounds the walk; a commit with more jobs than this is
	// reported as an error so the gate never decides on a truncated list.
	statusMaxPages = 10
)

// ErrTooManyStatuses means the commit has more statuses than one read walks.
var ErrTooManyStatuses = errors.New("gitlabapp: commit has too many statuses to read")

// CommitStatus is one job or external status on a commit. Status is
// GitLab's own value: created, pending, running, success, failed,
// canceled, skipped, manual and the like.
type CommitStatus struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	AllowFailure bool   `json:"allow_failure"`
}

// ListCommitStatuses returns the latest status per job name of sha in
// projectPath, read from the API for that exact commit. Redirects are not
// followed, so the token only ever reaches instanceURL's own host.
func (c *Client) ListCommitStatuses(ctx context.Context, instanceURL, accessToken, projectPath, sha string) ([]CommitStatus, error) {
	if c.HTTP == nil {
		return nil, errors.New("gitlabapp: no HTTP client configured")
	}
	noRedirect := *c.HTTP
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	rc := &Client{HTTP: &noRedirect}

	var all []CommitStatus
	for page := 1; page <= statusMaxPages; page++ {
		u := fmt.Sprintf("%s/projects/%s/repository/commits/%s/statuses?per_page=%d&page=%d",
			apiBaseURL(instanceURL), encodedProjectID(projectPath), url.PathEscape(sha), statusPageSize, page)
		var batch []CommitStatus
		if err := rc.do(ctx, http.MethodGet, u, "Bearer "+accessToken, nil, &batch); err != nil {
			return nil, err
		}
		all = append(all, batch...)
		if len(batch) < statusPageSize {
			return all, nil
		}
	}
	return nil, ErrTooManyStatuses
}
