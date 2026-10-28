package cigate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	githubPerPage     = 100
	githubMaxPages    = 5
	githubMaxBody     = 4 << 20
	githubAPIVersion  = "2022-11-28"
	rateLimitMinDelay = time.Second
)

// GitHub reads check runs and commit statuses from the GitHub REST API.
type GitHub struct {
	HTTP    *http.Client
	BaseURL string
	Token   string
	Owner   string
	Repo    string
	Now     func() time.Time
}

type ghCheckRuns struct {
	CheckRuns []struct {
		Name       string `json:"name"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"check_runs"`
}

type ghStatuses struct {
	Statuses []struct {
		Context string `json:"context"`
		State   string `json:"state"`
	} `json:"statuses"`
}

// Checks returns check runs and legacy commit statuses for sha.
func (g *GitHub) Checks(ctx context.Context, sha string) ([]Check, error) {
	var out []Check
	for page := 1; page <= githubMaxPages; page++ {
		var runs ghCheckRuns
		path := fmt.Sprintf("/repos/%s/%s/commits/%s/check-runs?per_page=%d&page=%d", url.PathEscape(g.Owner), url.PathEscape(g.Repo), url.PathEscape(sha), githubPerPage, page)
		if err := g.get(ctx, path, &runs); err != nil {
			return nil, err
		}
		for _, r := range runs.CheckRuns {
			out = append(out, Check{Name: r.Name, State: runState(r.Status, r.Conclusion)})
		}
		if len(runs.CheckRuns) < githubPerPage {
			break
		}
	}
	var st ghStatuses
	path := fmt.Sprintf("/repos/%s/%s/commits/%s/status?per_page=%d", url.PathEscape(g.Owner), url.PathEscape(g.Repo), url.PathEscape(sha), githubPerPage)
	if err := g.get(ctx, path, &st); err != nil {
		return nil, err
	}
	for _, s := range st.Statuses {
		out = append(out, Check{Name: s.Context, State: statusState(s.State)})
	}
	return out, nil
}

func runState(status, conclusion string) State {
	if status != "completed" {
		return Pending
	}
	switch conclusion {
	case "success", "neutral", "skipped":
		return Passed
	}
	return Failed
}

func statusState(state string) State {
	switch state {
	case "success":
		return Passed
	case "failure", "error":
		return Failed
	}
	return Pending
}

func (g *GitHub) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("cigate: build request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", githubAPIVersion)
	req.Header.Set("Authorization", "Bearer "+g.Token)
	if g.HTTP == nil {
		return errors.New("cigate: no HTTP client configured")
	}
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("cigate: read checks: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, githubMaxBody))
	if err != nil {
		return fmt.Errorf("cigate: read checks body: %w", err)
	}
	if rl := g.rateLimited(resp); rl != nil {
		return rl
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("cigate: read checks: GitHub returned status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("cigate: decode checks: %w", err)
	}
	return nil
}

// rateLimited maps a 403/429 or an exhausted quota to a RateLimitError.
func (g *GitHub) rateLimited(resp *http.Response) *RateLimitError {
	limited := resp.StatusCode == http.StatusTooManyRequests ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0") ||
		(resp.StatusCode == http.StatusForbidden && resp.Header.Get("Retry-After") != "")
	if !limited {
		return nil
	}
	if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
		return &RateLimitError{RetryAfter: time.Duration(s) * time.Second}
	}
	if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		now := time.Now
		if g.Now != nil {
			now = g.Now
		}
		if d := time.Unix(reset, 0).Sub(now()); d > rateLimitMinDelay {
			return &RateLimitError{RetryAfter: d}
		}
	}
	return &RateLimitError{RetryAfter: rateLimitMinDelay}
}
