// Command glinr-bot evaluates one pull request against .github/glinr-bot.yml.
// Run on pull_request_target with the base branch checked out; see docs/glinr-bot.md.
// Env: REPO, PR, GH_TOKEN, POLICY (default .github/glinr-bot.yml), MODE, APP_LOGIN.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	commentMarker = "<!-- glinr-bot -->"
	perPage       = 100
	maxPages      = 30
)

type ghPull struct {
	NodeID string `json:"node_id"`
	Title  string `json:"title"`
	Draft  bool   `json:"draft"`
	User   struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Head struct {
		SHA  string `json:"sha"`
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"head"`
	Base struct {
		Repo struct {
			FullName string `json:"full_name"`
		} `json:"repo"`
	} `json:"base"`
	Additions int `json:"additions"`
	Deletions int `json:"deletions"`
}

// client is a minimal GitHub REST and GraphQL client over net/http, so this
// helper starts no host process.
type client struct {
	base  string
	token string
	http  *http.Client
}

func newClient() *client {
	base := strings.TrimRight(os.Getenv("GITHUB_API_URL"), "/")
	if base == "" {
		base = "https://api.github.com"
	}
	return &client{base: base, token: os.Getenv("GH_TOKEN"), http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *client) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, body) //nolint:gosec // base is the GitHub API, path is built from the workflow's own env
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req) //nolint:gosec // base URL is the GitHub API, path is built from the workflow's own env
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read %s %s: %w", method, path, err)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s %s: %w", method, path, err)
		}
	}
	return nil
}

// pages walks a list endpoint until a short page, collecting every element.
func pages[T any](ctx context.Context, c *client, path string) ([]T, error) {
	var all []T
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for page := 1; page <= maxPages; page++ {
		var got []T
		if err := c.do(ctx, http.MethodGet, fmt.Sprintf("%s%sper_page=%d&page=%d", path, sep, perPage, page), nil, &got); err != nil {
			return nil, err
		}
		all = append(all, got...)
		if len(got) < perPage {
			break
		}
	}
	return all, nil
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "glinr-bot: "+format+"\n", a...)
	os.Exit(1)
}

func main() {
	repo, pr := os.Getenv("REPO"), os.Getenv("PR")
	if repo == "" || pr == "" {
		fail("REPO and PR are required")
	}
	path := os.Getenv("POLICY")
	if path == "" {
		path = ".github/glinr-bot.yml"
	}
	raw, err := os.ReadFile(path) //nolint:gosec // path comes from the workflow, not the PR
	if err != nil {
		fail("read policy: %v", err)
	}
	policy, err := ParsePolicy(raw)
	if err != nil {
		fail("%v", err)
	}
	if m := os.Getenv("MODE"); m != "" {
		policy.Mode = m
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c := newClient()
	var p ghPull
	if err := c.do(ctx, http.MethodGet, "/repos/"+repo+"/pulls/"+pr, nil, &p); err != nil {
		fail("%v", err)
	}
	files, err := pages[struct {
		Filename string `json:"filename"`
	}](ctx, c, "/repos/"+repo+"/pulls/"+pr+"/files")
	if err != nil {
		fail("%v", err)
	}
	info := PR{
		Author: p.User.Login, Title: p.Title, Draft: p.Draft,
		Fork:         p.Head.Repo.FullName != p.Base.Repo.FullName,
		ChangedLines: p.Additions + p.Deletions,
		SelfAuthored: p.User.Login == os.Getenv("APP_LOGIN") && p.User.Login != "",
	}
	for _, f := range files {
		info.Files = append(info.Files, f.Filename)
	}
	for _, l := range p.Labels {
		info.Labels = append(info.Labels, l.Name)
	}

	v := Evaluate(policy, info)
	var done []string
	if policy.Mode == ModeEnforce {
		done = act(ctx, c, repo, pr, p, v, info)
	}
	if err := upsertComment(ctx, c, repo, pr, renderComment(v, policy.Mode, done)); err != nil {
		fail("%v", err)
	}
	fmt.Printf("glinr-bot: %s#%s mode=%s rule=%q actions=%v done=%v\n", repo, pr, policy.Mode, v.Rule, v.Actions, done)
}

// act performs the earned actions that change anything besides the comment.
func act(ctx context.Context, c *client, repo, pr string, p ghPull, v Verdict, info PR) []string {
	var done []string
	for _, a := range v.Actions {
		switch a {
		case ActionApprove:
			if info.SelfAuthored {
				done = append(done, "approve skipped: the bot authored this PR")
				continue
			}
			if alreadyApproved(ctx, c, repo, pr, p.Head.SHA) {
				done = append(done, "approve: already approved this commit")
				continue
			}
			err := c.do(ctx, http.MethodPost, "/repos/"+repo+"/pulls/"+pr+"/reviews",
				map[string]string{"event": "APPROVE", "body": "Approved by glinr-bot: rule " + v.Rule + " passed."}, nil)
			if err != nil {
				done = append(done, "approve failed: "+err.Error())
				continue
			}
			done = append(done, "approved")
		case ActionAutomerge:
			q := map[string]any{
				"query":     "mutation($id: ID!) { enablePullRequestAutoMerge(input: {pullRequestId: $id, mergeMethod: SQUASH}) { clientMutationId } }",
				"variables": map[string]string{"id": p.NodeID},
			}
			var out struct {
				Errors []struct {
					Message string `json:"message"`
				} `json:"errors"`
			}
			if err := c.do(ctx, http.MethodPost, "/graphql", q, &out); err != nil {
				done = append(done, "auto-merge failed: "+err.Error())
				continue
			}
			if len(out.Errors) > 0 {
				done = append(done, "auto-merge failed: "+out.Errors[0].Message)
				continue
			}
			done = append(done, "auto-merge armed, GitHub merges when required checks pass")
		}
	}
	return done
}

func alreadyApproved(ctx context.Context, c *client, repo, pr, sha string) bool {
	reviews, err := pages[struct {
		State    string `json:"state"`
		CommitID string `json:"commit_id"`
		User     struct {
			Login string `json:"login"`
		} `json:"user"`
	}](ctx, c, "/repos/"+repo+"/pulls/"+pr+"/reviews")
	if err != nil {
		return false
	}
	for _, r := range reviews {
		if r.State == "APPROVED" && r.CommitID == sha && strings.HasSuffix(r.User.Login, "[bot]") {
			return true
		}
	}
	return false
}

func upsertComment(ctx context.Context, c *client, repo, pr, body string) error {
	comments, err := pages[struct {
		ID   int64  `json:"id"`
		Body string `json:"body"`
	}](ctx, c, "/repos/"+repo+"/issues/"+pr+"/comments")
	if err != nil {
		return err
	}
	payload := map[string]string{"body": body}
	for _, cm := range comments {
		if strings.Contains(cm.Body, commentMarker) {
			return c.do(ctx, http.MethodPatch, fmt.Sprintf("/repos/%s/issues/comments/%d", repo, cm.ID), payload, nil)
		}
	}
	return c.do(ctx, http.MethodPost, "/repos/"+repo+"/issues/"+pr+"/comments", payload, nil)
}

func renderComment(v Verdict, mode string, done []string) string {
	var b strings.Builder
	b.WriteString(commentMarker + "\n")
	b.WriteString("### glinr-bot\n\n")
	if v.Rule != "" {
		fmt.Fprintf(&b, "Rule **%s** matched. ", v.Rule)
	}
	b.WriteString(v.Reason + "\n\n")
	b.WriteString("| Gate | Result | Detail |\n| --- | --- | --- |\n")
	for _, g := range v.Gates {
		res := "pass"
		if !g.Pass {
			res = "fail"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", g.Name, res, g.Detail)
	}
	b.WriteString("\n")
	switch {
	case mode == ModeShadow && len(v.Actions) > 0 && v.Rule != "":
		fmt.Fprintf(&b, "Shadow mode: would %s. Nothing was changed.\n", strings.Join(withoutComment(v.Actions), " and "))
	case len(done) > 0:
		b.WriteString("Did: " + strings.Join(done, "; ") + ".\n")
	default:
		b.WriteString("No action taken. A person reviews this one.\n")
	}
	return b.String()
}

func withoutComment(actions []string) []string {
	var out []string
	for _, a := range actions {
		if a != ActionComment {
			out = append(out, a)
		}
	}
	if len(out) == 0 {
		return []string{"only comment"}
	}
	return out
}
