package repolayout

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Commit is one entry of a branch's history, reduced to what a deploy
// picker shows.
type Commit struct {
	SHA        string    `json:"sha"`
	Message    string    `json:"message"`
	Author     string    `json:"author"`
	AuthoredAt time.Time `json:"authored_at"`
	URL        string    `json:"url,omitempty"`
}

// CommitSource lists recent commits and branches through a provider API.
type CommitSource interface {
	Provider() string
	Commits(ctx context.Context, limit int) ([]Commit, error)
	Branches(ctx context.Context) ([]string, error)
}

const maxBranchPage = 100

// Provider names for CommitSource.Provider.
const (
	ProviderGitHub = "github"
	ProviderGitLab = "gitlab"
	ProviderGitea  = "gitea"
)

func (*githubSource) Provider() string { return ProviderGitHub }
func (*gitlabSource) Provider() string { return ProviderGitLab }
func (*giteaSource) Provider() string  { return ProviderGitea }

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func (s *githubSource) Commits(ctx context.Context, limit int) ([]Commit, error) {
	var raw []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
		Author *struct {
			Login string `json:"login"`
		} `json:"author"`
	}
	u := s.api + "/repos/" + escapePath(s.repo) + "/commits?per_page=" + strconv.Itoa(limit) + "&sha=" + url.QueryEscape(s.ref)
	if err := getJSON(ctx, s.c, u, s.headers("application/vnd.github+json"), &raw); err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(raw))
	for _, c := range raw {
		author := c.Commit.Author.Name
		if author == "" && c.Author != nil {
			author = c.Author.Login
		}
		out = append(out, Commit{SHA: c.SHA, Message: firstLine(c.Commit.Message), Author: author, AuthoredAt: parseTime(c.Commit.Author.Date), URL: c.HTMLURL})
	}
	return out, nil
}

func (s *githubSource) Branches(ctx context.Context) ([]string, error) {
	var raw []struct {
		Name string `json:"name"`
	}
	u := s.api + "/repos/" + escapePath(s.repo) + "/branches?per_page=" + strconv.Itoa(maxBranchPage)
	if err := getJSON(ctx, s.c, u, s.headers("application/vnd.github+json"), &raw); err != nil {
		return nil, err
	}
	return branchNames(raw), nil
}

func (s *gitlabSource) Commits(ctx context.Context, limit int) ([]Commit, error) {
	var raw []struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		AuthorName   string `json:"author_name"`
		AuthoredDate string `json:"authored_date"`
		WebURL       string `json:"web_url"`
	}
	u := s.api + "/projects/" + url.PathEscape(s.repo) + "/repository/commits?per_page=" + strconv.Itoa(limit) + "&ref_name=" + url.QueryEscape(s.ref)
	if err := getJSON(ctx, s.c, u, s.headers(), &raw); err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(raw))
	for _, c := range raw {
		out = append(out, Commit{SHA: c.ID, Message: firstLine(c.Title), Author: c.AuthorName, AuthoredAt: parseTime(c.AuthoredDate), URL: c.WebURL})
	}
	return out, nil
}

func (s *gitlabSource) Branches(ctx context.Context) ([]string, error) {
	var raw []struct {
		Name string `json:"name"`
	}
	u := s.api + "/projects/" + url.PathEscape(s.repo) + "/repository/branches?per_page=" + strconv.Itoa(maxBranchPage)
	if err := getJSON(ctx, s.c, u, s.headers(), &raw); err != nil {
		return nil, err
	}
	return branchNames(raw), nil
}

func (s *giteaSource) Commits(ctx context.Context, limit int) ([]Commit, error) {
	var raw []struct {
		SHA     string `json:"sha"`
		HTMLURL string `json:"html_url"`
		Commit  struct {
			Message string `json:"message"`
			Author  struct {
				Name string `json:"name"`
				Date string `json:"date"`
			} `json:"author"`
		} `json:"commit"`
	}
	u := s.api + "/repos/" + escapePath(s.repo) + "/commits?stat=false&verification=false&files=false&limit=" + strconv.Itoa(limit) + "&sha=" + url.QueryEscape(s.ref)
	if err := getJSON(ctx, s.c, u, s.headers(), &raw); err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(raw))
	for _, c := range raw {
		out = append(out, Commit{SHA: c.SHA, Message: firstLine(c.Commit.Message), Author: c.Commit.Author.Name, AuthoredAt: parseTime(c.Commit.Author.Date), URL: c.HTMLURL})
	}
	return out, nil
}

func (s *giteaSource) Branches(ctx context.Context) ([]string, error) {
	var raw []struct {
		Name string `json:"name"`
	}
	u := s.api + "/repos/" + escapePath(s.repo) + "/branches?limit=" + strconv.Itoa(maxBranchPage)
	if err := getJSON(ctx, s.c, u, s.headers(), &raw); err != nil {
		return nil, err
	}
	return branchNames(raw), nil
}

func branchNames(raw []struct {
	Name string `json:"name"`
}) []string {
	out := make([]string, 0, len(raw))
	for _, b := range raw {
		if b.Name != "" {
			out = append(out, b.Name)
		}
	}
	return out
}
