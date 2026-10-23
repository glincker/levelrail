// Package cliupdate lets the CLI find a newer release and replace its own
// executable with it, reusing selfupgrade's download, checksum and signature code.
package cliupdate

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

	"github.com/GLINCKER/levelrail/kit/semver"
	"github.com/GLINCKER/levelrail/kit/upgrade"
)

const (
	listLimit     = 30
	metadataBytes = 4 << 20
)

// Source says where releases are published and which asset belongs to this build.
type Source struct {
	// Repo is "owner/name".
	Repo string
	// APIBase serves the releases listing, for example https://api.github.com.
	APIBase string
	// DownloadBase is the asset prefix, the tag is appended by the fetcher.
	DownloadBase string
	// Asset is the release asset for this OS and arch.
	Asset  string
	Client *http.Client
	// AllowHTTP permits plain http bases, for tests against a local server.
	AllowHTTP bool
}

// Asset names the CLI release asset the way release.yml does.
func Asset(base, goos, goarch string) string {
	return base + "-" + goos + "-" + goarch
}

type rawRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	Assets     []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

// Latest returns the highest stable tag that ships s.Asset, falling back to
// the highest prerelease when there is no stable one (install-cli.sh's rule).
// An empty tag with a nil error means nothing suitable is published.
func (s Source) Latest(ctx context.Context) (string, error) {
	u, err := url.Parse(s.APIBase)
	if err != nil || u.Host == "" || (u.Scheme != "https" && (!s.AllowHTTP || u.Scheme != "http")) {
		return "", errors.New("release API base URL must be https")
	}
	endpoint := strings.TrimRight(s.APIBase, "/") + "/repos/" + s.Repo + "/releases?per_page=" + fmt.Sprint(listLimit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("list releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("list releases: status %d", resp.StatusCode)
	}
	var rrs []rawRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, metadataBytes)).Decode(&rrs); err != nil {
		return "", fmt.Errorf("decode releases: %w", err)
	}
	if tag := s.pick(rrs, false); tag != "" {
		return tag, nil
	}
	return s.pick(rrs, true), nil
}

func (s Source) pick(rrs []rawRelease, prerelease bool) string {
	best := ""
	for _, rr := range rrs {
		if rr.Prerelease != prerelease || rr.Draft || !upgrade.IsProductTag(rr.TagName) || !hasAsset(rr, s.Asset) {
			continue
		}
		if _, ok := semver.Compare(rr.TagName, rr.TagName); !ok {
			continue
		}
		if c, _ := semver.Compare(rr.TagName, best); best == "" || c > 0 {
			best = rr.TagName
		}
	}
	return best
}

func hasAsset(rr rawRelease, name string) bool {
	for _, a := range rr.Assets {
		if a.Name == name {
			return true
		}
	}
	return false
}

// Newer reports whether latest is a higher release than current. A dev build
// or an unparsable version never reports newer.
func Newer(current, latest string) bool {
	if latest == "" || current == "dev" {
		return false
	}
	c, ok := semver.Compare(current, latest)
	return ok && c < 0
}
