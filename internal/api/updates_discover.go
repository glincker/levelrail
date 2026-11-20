package api

import (
	"context"
	"net/url"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

const (
	notesSourceFallback  = "fallback"
	envReleaseBaseForWeb = "LEVELRAIL_RELEASE_BASE_URL"
)

// discoverTag is swapped in tests; production reads the public release pages.
var discoverTag = func(ctx context.Context) (selfupgrade.Discovered, error) {
	d := selfupgrade.Discoverer{Repo: githubRepo}
	if u, err := url.Parse(strings.TrimSpace(os.Getenv(envReleaseBaseForWeb))); err == nil && u.Hostname() != "" {
		d.Hosts = []string{strings.ToLower(u.Hostname())}
	}
	return d.Latest(ctx)
}

// discoverStableRelease names the newest stable tag when the releases API
// cannot be reached. The result has no notes and no asset list, so every
// consumer must treat the notes as unavailable; verification is unchanged.
func discoverStableRelease(ctx context.Context) (*githubRelease, error) {
	found, err := discoverTag(ctx)
	if err != nil {
		return nil, err
	}
	return &githubRelease{TagName: found.Tag, HTMLURL: found.URL, PublishedAt: found.PublishedAt, Fallback: true}, nil
}
