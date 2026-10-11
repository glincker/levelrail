package apiclient

import (
	"context"
	"net/http"
	"net/url"
	"time"
)

// GitCommit is one commit of the connected branch with its deploy outcome.
type GitCommit struct {
	SHA          string    `json:"sha"`
	Message      string    `json:"message"`
	Author       string    `json:"author"`
	AuthoredAt   time.Time `json:"authored_at"`
	URL          string    `json:"url,omitempty"`
	DeployStatus string    `json:"deploy_status,omitempty"`
	DeployID     string    `json:"deploy_id,omitempty"`
}

// DeploySuggestion is one computed next step for an app's deploys.
type DeploySuggestion struct {
	ID       string            `json:"id"`
	Severity string            `json:"severity"`
	Reason   string            `json:"reason"`
	Params   map[string]string `json:"params,omitempty"`
}

// GitSourceCommits mirrors GET /api/v1/apps/{name}/git-source/commits.
type GitSourceCommits struct {
	Branch          string             `json:"branch"`
	ConnectedBranch string             `json:"connected_branch"`
	Provider        string             `json:"provider"`
	Head            *GitCommit         `json:"head,omitempty"`
	Commits         []GitCommit        `json:"commits"`
	Branches        []string           `json:"branches"`
	AheadBy         *int               `json:"ahead_by,omitempty"`
	UpToDate        bool               `json:"up_to_date"`
	Suggestions     []DeploySuggestion `json:"suggestions"`
	Degraded        bool               `json:"degraded"`
	BuildType       string             `json:"build_type"`
	Build           struct {
		Summary string `json:"summary"`
	} `json:"build"`
}

// GitSourceCommits lists recent commits of a branch with deploy status and
// suggestions. branch empty means the connected branch.
func (c *Client) GitSourceCommits(ctx context.Context, name, branch string, refresh bool) (GitSourceCommits, error) {
	q := url.Values{}
	if branch != "" {
		q.Set("branch", branch)
	}
	if refresh {
		q.Set("refresh", "1")
	}
	path := "/api/v1/apps/" + PathEscape(name) + "/git-source/commits"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out GitSourceCommits
	err := c.do(ctx, http.MethodGet, path, nil, &out)
	return out, err
}
