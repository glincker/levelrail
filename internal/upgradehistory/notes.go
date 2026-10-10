package upgradehistory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	notesAPIHost    = "api.github.com"
	notesTimeout    = 10 * time.Second
	notesMaxBody    = 256 << 10
	notesMaxRunes   = 6000
	notesMaxRedirts = 3
)

// NotesFetcher returns the release notes text for a tag. Errors are normal
// (offline, rate limited) and leave the snapshot unavailable.
type NotesFetcher func(ctx context.Context, tag string) (string, error)

// CleanNotes strips HTML comments from release notes, the same cleanup the
// Updates page applies before rendering.
func CleanNotes(body string) string {
	var b strings.Builder
	rest := body
	for {
		start := strings.Index(rest, "<!--")
		if start < 0 {
			b.WriteString(rest)
			break
		}
		b.WriteString(rest[:start])
		end := strings.Index(rest[start:], "-->")
		if end < 0 {
			break
		}
		rest = rest[start+end+len("-->"):]
	}
	return strings.TrimSpace(b.String())
}

func boundNotes(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' && r != '\t' || r == 0x7f {
			return -1
		}
		return r
	}, strings.ToValidUTF8(CleanNotes(s), ""))
	if utf8.RuneCountInString(s) <= notesMaxRunes {
		return s
	}
	return string([]rune(s)[:notesMaxRunes])
}

// NewGitHubNotesFetcher fetches a tag's release body from the GitHub API for
// repoSlug ("owner/name"). It refuses redirects that leave api.github.com
// over https and caps the response size.
func NewGitHubNotesFetcher(repoSlug string) NotesFetcher {
	client := &http.Client{
		Timeout: notesTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= notesMaxRedirts {
				return errors.New("too many redirects")
			}
			if req.URL.Scheme != "https" || req.URL.Hostname() != notesAPIHost {
				return fmt.Errorf("redirect to %s refused", req.URL.Host)
			}
			return nil
		},
	}
	return func(ctx context.Context, tag string) (string, error) {
		if repoSlug == "" || !IsRelease(tag) {
			return "", errors.New("no release notes source for this version")
		}
		u := "https://" + notesAPIHost + "/repos/" + repoSlug + "/releases/tags/" + url.PathEscape(tag)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return "", fmt.Errorf("build release notes request: %w", err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("fetch release notes for %s: %w", tag, err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("release notes for %s: status %d", tag, resp.StatusCode)
		}
		var body struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, notesMaxBody)).Decode(&body); err != nil {
			return "", fmt.Errorf("decode release notes for %s: %w", tag, err)
		}
		return boundNotes(body.Body), nil
	}
}
