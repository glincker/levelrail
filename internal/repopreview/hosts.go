package repopreview

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/repolayout"
)

const bodyLimit = 2 << 20

// HTTPClient reads repository metadata through provider REST APIs. The
// http.Client must be the SSRF-guarded one; hosts are re-checked by Builder.
type HTTPClient struct {
	HTTP  *http.Client
	Hosts func(ctx context.Context) repolayout.Hosts
}

type apiKind int

const (
	kindNone apiKind = iota
	kindGitHub
	kindGitLab
	kindGitea
)

func (c *HTTPClient) kindFor(ctx context.Context, host string) (apiKind, string) {
	var h repolayout.Hosts
	if c.Hosts != nil {
		h = c.Hosts(ctx)
	}
	same := func(a string) bool { return a != "" && strings.EqualFold(a, host) }
	switch {
	case host == "github.com" || host == "www.github.com":
		return kindGitHub, "https://api.github.com"
	case same(h.GitHub):
		return kindGitHub, "https://" + host + "/api/v3"
	case host == "gitlab.com" || strings.HasPrefix(host, "gitlab.") || same(h.GitLab):
		return kindGitLab, "https://" + host + "/api/v4"
	case host == "codeberg.org" || strings.HasPrefix(host, "gitea.") || strings.HasPrefix(host, "forgejo.") || same(h.Gitea):
		return kindGitea, "https://" + host + "/api/v1"
	}
	return kindNone, ""
}

// Fetch implements HostClient.
func (c *HTTPClient) Fetch(ctx context.Context, t Target, token string) (Meta, error) {
	kind, api := c.kindFor(ctx, t.Host)
	switch kind {
	case kindGitHub:
		return c.github(ctx, api, t, token)
	case kindGitLab:
		return c.gitlab(ctx, api, t, token)
	case kindGitea:
		return c.gitea(ctx, api, t, token)
	}
	return Meta{}, ErrNoMetadata
}

func (c *HTTPClient) get(ctx context.Context, rawURL, authValue string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if authValue != "" {
		req.Header.Set("Authorization", authValue)
	}
	resp, err := c.HTTP.Do(req) //nolint:gosec // host is allowlisted by Builder and the client is SSRF-guarded
	if err != nil {
		return fmt.Errorf("get %s: %w", req.URL.Redacted(), err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusTooManyRequests,
		resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		return ErrRateLimited
	case resp.StatusCode == http.StatusNotFound, resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return ErrNotFound
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		return fmt.Errorf("provider api returned %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, bodyLimit))
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if s, ok := out.(*string); ok {
		*s = string(b)
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func bearer(token string) string {
	if token == "" {
		return ""
	}
	return "Bearer " + token
}

type ghRepo struct {
	Name          string                 `json:"name"`
	Owner         struct{ Login string } `json:"owner"`
	Description   string                 `json:"description"`
	DefaultBranch string                 `json:"default_branch"`
	Private       bool                   `json:"private"`
	Stars         int                    `json:"stargazers_count"`
	Forks         int                    `json:"forks_count"`
	Size          int                    `json:"size"`
	Topics        []string               `json:"topics"`
	License       *struct{ Name string } `json:"license"`
}

func (c *HTTPClient) github(ctx context.Context, api string, t Target, token string) (Meta, error) {
	base := api + "/repos/" + escape(t.Owner+"/"+t.Repo)
	auth := bearer(token)
	var r ghRepo
	if err := c.get(ctx, base, auth, &r); err != nil {
		return Meta{}, err
	}
	m := Meta{Owner: r.Owner.Login, Name: r.Name, Description: r.Description, DefaultBranch: r.DefaultBranch,
		Private: r.Private, Stars: &r.Stars, Forks: &r.Forks, Topics: r.Topics}
	m.SizeKB = &r.Size
	if r.License != nil {
		m.License = r.License.Name
	}
	ref := t.Ref
	if ref == "" {
		ref = r.DefaultBranch
	}
	_ = c.get(ctx, base+"/languages", auth, &m.Languages)
	var branches []struct{ Name string }
	if c.get(ctx, base+"/branches?per_page="+fmt.Sprint(envInt(EnvMaxBranches, 50)), auth, &branches) == nil {
		for _, b := range branches {
			m.Branches = append(m.Branches, b.Name)
		}
	}
	var commits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if c.get(ctx, base+"/commits?per_page=1&sha="+url.QueryEscape(ref), auth, &commits) == nil && len(commits) > 0 {
		k := commits[0]
		m.LastCommit = &Commit{SHA: k.SHA, Message: k.Commit.Message, Author: k.Commit.Author.Name, At: k.Commit.Author.Date}
	}
	var readme string
	_ = c.get(ctx, base+"/readme?ref="+url.QueryEscape(ref), auth, &readmeRaw{&readme})
	m.Readme = readme
	return m, nil
}

type readmeRaw struct{ out *string }

func (r *readmeRaw) UnmarshalJSON(b []byte) error {
	var v struct {
		Content  string `json:"content"`
		Encoding string `json:"encoding"`
	}
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	if v.Encoding == "base64" {
		*r.out = decodeBase64(v.Content, envInt(EnvReadmeChars, 400)*8)
	}
	return nil
}

type glProject struct {
	Name          string   `json:"name"`
	Path          string   `json:"path_with_namespace"`
	Description   string   `json:"description"`
	DefaultBranch string   `json:"default_branch"`
	Visibility    string   `json:"visibility"`
	Stars         int      `json:"star_count"`
	Forks         int      `json:"forks_count"`
	Topics        []string `json:"topics"`
	ReadmeURL     string   `json:"readme_url"`
}

func (c *HTTPClient) gitlab(ctx context.Context, api string, t Target, token string) (Meta, error) {
	base := api + "/projects/" + url.PathEscape(t.Owner+"/"+t.Repo)
	auth := bearer(token)
	var p glProject
	if err := c.get(ctx, base, auth, &p); err != nil {
		return Meta{}, err
	}
	m := Meta{Owner: t.Owner, Name: p.Name, Description: p.Description, DefaultBranch: p.DefaultBranch,
		Private: p.Visibility != "public", Stars: &p.Stars, Forks: &p.Forks, Topics: p.Topics}
	ref := t.Ref
	if ref == "" {
		ref = p.DefaultBranch
	}
	var pct map[string]float64
	if c.get(ctx, base+"/languages", auth, &pct) == nil {
		m.Languages = map[string]int64{}
		for k, v := range pct {
			m.Languages[k] = int64(v * 100)
		}
	}
	var branches []struct{ Name string }
	if c.get(ctx, base+"/repository/branches?per_page="+fmt.Sprint(envInt(EnvMaxBranches, 50)), auth, &branches) == nil {
		for _, b := range branches {
			m.Branches = append(m.Branches, b.Name)
		}
	}
	var commits []struct {
		ID      string    `json:"id"`
		Message string    `json:"message"`
		Author  string    `json:"author_name"`
		At      time.Time `json:"committed_date"`
	}
	if c.get(ctx, base+"/repository/commits?per_page=1&ref_name="+url.QueryEscape(ref), auth, &commits) == nil && len(commits) > 0 {
		k := commits[0]
		m.LastCommit = &Commit{SHA: k.ID, Message: k.Message, Author: k.Author, At: k.At}
	}
	if p.ReadmeURL != "" {
		parts := strings.SplitN(p.ReadmeURL, "/blob/", 2)
		if len(parts) == 2 {
			refPath := strings.SplitN(parts[1], "/", 2)
			if len(refPath) == 2 {
				var raw string
				_ = c.get(ctx, base+"/repository/files/"+url.PathEscape(refPath[1])+"/raw?ref="+url.QueryEscape(ref), auth, &raw)
				m.Readme = raw
			}
		}
	}
	return m, nil
}

func (c *HTTPClient) gitea(ctx context.Context, api string, t Target, token string) (Meta, error) {
	base := api + "/repos/" + escape(t.Owner+"/"+t.Repo)
	auth := ""
	if token != "" {
		auth = "token " + token
	}
	var r struct {
		Name          string                 `json:"name"`
		Owner         struct{ Login string } `json:"owner"`
		Description   string                 `json:"description"`
		DefaultBranch string                 `json:"default_branch"`
		Private       bool                   `json:"private"`
		Stars         int                    `json:"stars_count"`
		Forks         int                    `json:"forks_count"`
		Size          int                    `json:"size"`
	}
	if err := c.get(ctx, base, auth, &r); err != nil {
		return Meta{}, err
	}
	m := Meta{Owner: r.Owner.Login, Name: r.Name, Description: r.Description, DefaultBranch: r.DefaultBranch,
		Private: r.Private, Stars: &r.Stars, Forks: &r.Forks, SizeKB: &r.Size}
	ref := t.Ref
	if ref == "" {
		ref = r.DefaultBranch
	}
	_ = c.get(ctx, base+"/languages", auth, &m.Languages)
	var branches []struct{ Name string }
	if c.get(ctx, base+"/branches?limit="+fmt.Sprint(envInt(EnvMaxBranches, 50)), auth, &branches) == nil {
		for _, b := range branches {
			m.Branches = append(m.Branches, b.Name)
		}
	}
	var commits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
			Author  struct {
				Name string    `json:"name"`
				Date time.Time `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	if c.get(ctx, base+"/commits?limit=1&sha="+url.QueryEscape(ref), auth, &commits) == nil && len(commits) > 0 {
		k := commits[0]
		m.LastCommit = &Commit{SHA: k.SHA, Message: k.Commit.Message, Author: k.Commit.Author.Name, At: k.Commit.Author.Date}
	}
	var raw string
	_ = c.get(ctx, base+"/raw/README.md?ref="+url.QueryEscape(ref), auth, &raw)
	m.Readme = raw
	return m, nil
}

func decodeBase64(s string, limit int) string {
	b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(s, "\n", ""))
	if err != nil {
		return ""
	}
	if len(b) > limit {
		b = b[:limit]
	}
	return string(b)
}

func escape(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}
