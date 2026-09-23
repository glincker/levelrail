package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Client is a minimal GitHub REST client for the calls this tool needs.
type Client struct {
	HTTP  *http.Client
	Base  string
	Token string
	Repo  string
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Body        string    `json:"body"`
	Draft       bool      `json:"draft"`
	Prerelease  bool      `json:"prerelease"`
	PublishedAt time.Time `json:"published_at"`
	Assets      []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

type ghUser struct {
	Login string `json:"login"`
}

type ghCompare struct {
	Commits []struct {
		SHA    string  `json:"sha"`
		Author *ghUser `json:"author"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	} `json:"commits"`
}

type ghPull struct {
	Body   string `json:"body"`
	User   ghUser `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

func (c *Client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		buf, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode %s: %w", path, err)
		}
		body = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Base+path, body)
	if err != nil {
		return fmt.Errorf("build request %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, msg)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	return nil
}

// Releases returns published (non-draft) releases, oldest first.
func (c *Client) Releases(ctx context.Context) ([]ghRelease, error) {
	var all []ghRelease
	for page := 1; ; page++ {
		var batch []ghRelease
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/releases?per_page=100&page=%d", c.Repo, page), nil, &batch); err != nil {
			return nil, err
		}
		for _, r := range batch {
			if !r.Draft {
				all = append(all, r)
			}
		}
		if len(batch) < 100 {
			break
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].PublishedAt.Before(all[j].PublishedAt) })
	return all, nil
}

// Changes lists the commits between two tags, enriched with PR metadata.
func (c *Client) Changes(ctx context.Context, prev, tag string) ([]Change, error) {
	var cmp ghCompare
	if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/compare/%s...%s?per_page=250", c.Repo, prev, tag), nil, &cmp); err != nil {
		return nil, err
	}
	var out []Change
	for _, raw := range cmp.Commits {
		ch := ParseCommit(raw.SHA, raw.Commit.Message)
		if raw.Author != nil {
			ch.Author = raw.Author.Login
		}
		if ch.PR > 0 {
			var pr ghPull
			if err := c.do(ctx, http.MethodGet, fmt.Sprintf("/repos/%s/pulls/%d", c.Repo, ch.PR), nil, &pr); err != nil {
				return nil, err
			}
			labels := make([]string, 0, len(pr.Labels))
			for _, l := range pr.Labels {
				labels = append(labels, l.Name)
			}
			ch.ApplyPR(pr.Body, pr.User.Login, labels)
		}
		out = append(out, ch)
	}
	return out, nil
}

var newContributorRe = regexp.MustCompile(`(?m)^\* @(\S+) made their first contribution in \S+/pull/(\d+)`)

// NewContributors asks GitHub's generate-notes endpoint who contributed
// for the first time in this range, since that needs full repo history.
func (c *Client) NewContributors(ctx context.Context, prev, tag string) ([]Contributor, error) {
	in := map[string]string{"tag_name": tag}
	if prev != "" {
		in["previous_tag_name"] = prev
	}
	var out struct {
		Body string `json:"body"`
	}
	if err := c.do(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/releases/generate-notes", c.Repo), in, &out); err != nil {
		return nil, err
	}
	return ParseNewContributors(out.Body), nil
}

// ParseNewContributors extracts the "New Contributors" list GitHub renders.
func ParseNewContributors(body string) []Contributor {
	var out []Contributor
	for _, m := range newContributorRe.FindAllStringSubmatch(body, -1) {
		n, _ := strconv.Atoi(m[2])
		out = append(out, Contributor{Login: m[1], PR: n})
	}
	return out
}

// Contributors returns the distinct human authors of the changes, sorted.
func Contributors(changes []Change) []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range changes {
		if c.Author == "" || strings.HasSuffix(c.Author, "[bot]") || seen[c.Author] {
			continue
		}
		seen[c.Author] = true
		out = append(out, c.Author)
	}
	sort.Strings(out)
	return out
}
