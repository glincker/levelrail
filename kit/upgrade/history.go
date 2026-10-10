package upgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"time"
)

// ManifestAsset is the per-release file the release workflow publishes with
// the binary's own `version --json` output, so a release's schema version is
// known without downloading and running it.
const ManifestAsset = "release-manifest.json"

// ChannelAll is the history view that mixes stable and pre-releases.
const ChannelAll = "all"

const (
	manifestMaxBytes = 64 << 10
	githubDownload   = "github.com"
)

// HistoryAsset is one downloadable file on a release.
type HistoryAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// HistoryRelease is one published GitHub release, with the asset details a
// rollback plan needs.
type HistoryRelease struct {
	Tag         string         `json:"tag"`
	URL         string         `json:"url"`
	PublishedAt string         `json:"published_at"`
	Body        string         `json:"body"`
	Prerelease  bool           `json:"prerelease"`
	Assets      []HistoryAsset `json:"assets"`
}

// Asset returns the asset called name.
func (r HistoryRelease) Asset(name string) (HistoryAsset, bool) {
	for _, a := range r.Assets {
		if a.Name == name {
			return a, true
		}
	}
	return HistoryAsset{}, false
}

// AssetNames lists every asset name, for Run's Inputs.
func (r HistoryRelease) AssetNames() []string {
	out := make([]string, 0, len(r.Assets))
	for _, a := range r.Assets {
		out = append(out, a.Name)
	}
	return out
}

type rawHistory struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
	Prerelease  bool   `json:"prerelease"`
	Draft       bool   `json:"draft"`
	Assets      []struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// selectHistory keeps non-draft releases that belong to channel (stable:
// non-prerelease, beta: prerelease, all: both), newest published first, at
// most limit. Order comes from published_at because GitHub's list order is
// by internal id and is not reliable.
func selectHistory(rrs []rawHistory, channel string, limit int) []HistoryRelease {
	type dated struct {
		at  time.Time
		rel HistoryRelease
	}
	var keep []dated
	for _, rr := range rrs {
		if rr.Draft {
			continue
		}
		switch channel {
		case ChannelStable:
			if rr.Prerelease {
				continue
			}
		case ChannelBeta:
			if !rr.Prerelease {
				continue
			}
		}
		at, err := time.Parse(time.RFC3339, rr.PublishedAt)
		if err != nil {
			continue
		}
		rel := HistoryRelease{Tag: rr.TagName, URL: rr.HTMLURL, PublishedAt: rr.PublishedAt, Body: rr.Body, Prerelease: rr.Prerelease}
		for _, a := range rr.Assets {
			rel.Assets = append(rel.Assets, HistoryAsset{Name: a.Name, Size: a.Size, URL: a.URL})
		}
		keep = append(keep, dated{at, rel})
	}
	sort.SliceStable(keep, func(i, j int) bool { return keep[i].at.After(keep[j].at) })
	if limit > 0 && len(keep) > limit {
		keep = keep[:limit]
	}
	out := make([]HistoryRelease, 0, len(keep))
	for _, k := range keep {
		out = append(out, k.rel)
	}
	return out
}

// FetchHistory lists the newest limit releases of repo for channel. An empty
// result with a nil error means GitHub has none for that channel.
func FetchHistory(ctx context.Context, repo, channel string, limit int) ([]HistoryRelease, error) {
	var rrs []rawHistory
	status, err := githubGet(ctx, "https://api.github.com/repos/"+repo+"/releases?per_page=50", &rrs)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return []HistoryRelease{}, nil
	}
	return selectHistory(rrs, channel, limit), nil
}

// Manifest is the content of ManifestAsset.
type Manifest struct {
	Version       string `json:"version"`
	SchemaVersion int    `json:"schema_version"`
}

// FetchManifest downloads and decodes a release manifest. Only https URLs on
// github.com are fetched, and the body is size-capped.
func FetchManifest(ctx context.Context, rawURL string) (Manifest, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" || u.Hostname() != githubDownload {
		return Manifest{}, fmt.Errorf("upgrade: manifest url %q is not a github.com https url", rawURL)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Manifest{}, fmt.Errorf("upgrade: build manifest request: %w", err)
	}
	resp, err := channelHTTPClient.Do(req)
	if err != nil {
		return Manifest{}, fmt.Errorf("upgrade: fetch manifest: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Manifest{}, fmt.Errorf("upgrade: manifest status %d", resp.StatusCode)
	}
	var m Manifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, manifestMaxBytes)).Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("upgrade: decode manifest: %w", err)
	}
	return m, nil
}

// SchemaVerdict says whether a target binary can run on the current database.
type SchemaVerdict string

// Verdicts for a rollback or re-roll against the database's schema version.
const (
	// VerdictBinaryOnly: the target supports the database as it is (equal).
	VerdictBinaryOnly SchemaVerdict = "binary_only"
	// VerdictForward: the target is newer and will migrate the database.
	VerdictForward SchemaVerdict = "forward"
	// VerdictRestoreRequired: the target is older than the database and would
	// refuse to start; only a backup restore (losing newer data) works.
	VerdictRestoreRequired SchemaVerdict = "restore_required"
	// VerdictUnknown: the target's schema version could not be determined.
	VerdictUnknown SchemaVerdict = "unknown"
)

// VerdictFor compares the database's schema version with the target binary's.
// A negative value means unknown on that side.
func VerdictFor(dbVersion, targetVersion int) SchemaVerdict {
	switch {
	case dbVersion < 0 || targetVersion < 0:
		return VerdictUnknown
	case targetVersion == dbVersion:
		return VerdictBinaryOnly
	case targetVersion > dbVersion:
		return VerdictForward
	default:
		return VerdictRestoreRequired
	}
}
