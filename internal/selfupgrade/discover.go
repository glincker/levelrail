package selfupgrade

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/netclient"
	"github.com/GLINCKER/levelrail/kit/semver"
)

const (
	// DefaultWebBase serves the release pages and feed that need no API access.
	DefaultWebBase   = "https://github.com"
	discoverMaxBytes = 2 << 20
	discoverTimeout  = 15 * time.Second
	discoverRedirect = 3
	tagPathMarker    = "/releases/tag/"
)

// ErrNoRelease is returned when no source named a usable release tag.
var ErrNoRelease = errors.New("no release tag could be learned from the web sources")

// Discovered is a release tag learned without the releases API. It carries no
// release notes, so callers must treat the notes as unavailable.
type Discovered struct {
	Tag         string
	URL         string
	PublishedAt string
	// Source is "atom" or "redirect".
	Source string
}

// Discoverer finds the newest stable tag from the public release pages. It
// only names a tag: downloading and verifying the release is unchanged.
type Discoverer struct {
	Repo    string
	WebBase string
	Client  *http.Client
	// Hosts are extra hosts a redirect may reach, besides the WebBase host.
	Hosts []string
	// AllowHTTP permits a plain-http WebBase, for tests against a local server.
	AllowHTTP bool
	MaxBytes  int64
	Timeout   time.Duration
}

type atomFeed struct {
	Entries []struct {
		Updated string `xml:"updated"`
		Links   []struct {
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

// Latest tries the releases feed, which also gives the publish time, then the
// releases/latest redirect.
func (d Discoverer) Latest(ctx context.Context) (Discovered, error) {
	base := d.WebBase
	if base == "" {
		base = DefaultWebBase
	}
	bu, err := url.Parse(base)
	if err != nil || bu.Host == "" || (bu.Scheme != "https" && (!d.AllowHTTP || bu.Scheme != "http")) {
		return Discovered{}, errors.New("web base URL must be https")
	}
	if d.Repo == "" || strings.Count(d.Repo, "/") != 1 || strings.ContainsAny(d.Repo, " ?#\\") {
		return Discovered{}, fmt.Errorf("repository %q must be owner/name", d.Repo)
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = discoverTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	root := strings.TrimRight(base, "/") + "/" + d.Repo
	hosts := append([]string{strings.ToLower(bu.Hostname())}, d.Hosts...)

	found, atomErr := d.fromAtom(ctx, root+"/releases.atom", hosts)
	if atomErr == nil {
		return found, nil
	}
	found, redirErr := d.fromRedirect(ctx, root+"/releases/latest", root+tagPathMarker, hosts)
	if redirErr == nil {
		return found, nil
	}
	return Discovered{}, fmt.Errorf("%w (feed: %w; redirect: %w)", ErrNoRelease, atomErr, redirErr)
}

func (d Discoverer) client(hosts []string, follow bool) *http.Client {
	c := d.Client
	if c == nil {
		c = netclient.New()
	}
	cp := *c
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if !follow {
			return http.ErrUseLastResponse
		}
		if len(via) >= discoverRedirect {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" && !d.AllowHTTP {
			return fmt.Errorf("redirect to %s refused: not https", req.URL.Host)
		}
		if !hostAllowed(req.URL.Hostname(), hosts) {
			return fmt.Errorf("redirect to %s refused: host not allowed", req.URL.Host)
		}
		return nil
	}
	return &cp
}

func (d Discoverer) fromAtom(ctx context.Context, feedURL string, hosts []string) (Discovered, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return Discovered{}, fmt.Errorf("build feed request: %w", err)
	}
	resp, err := d.client(hosts, true).Do(req)
	if err != nil {
		return Discovered{}, fmt.Errorf("fetch release feed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Discovered{}, fmt.Errorf("release feed: status %d", resp.StatusCode)
	}
	limit := d.MaxBytes
	if limit <= 0 {
		limit = discoverMaxBytes
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return Discovered{}, fmt.Errorf("read release feed: %w", err)
	}
	if int64(len(raw)) > limit {
		return Discovered{}, fmt.Errorf("release feed exceeds %d bytes", limit)
	}
	var feed atomFeed
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return Discovered{}, fmt.Errorf("decode release feed: %w", err)
	}
	var best Discovered
	for _, e := range feed.Entries {
		for _, l := range e.Links {
			_, tag, ok := strings.Cut(l.Href, tagPathMarker)
			if !ok || !stableProductTag(tag) {
				continue
			}
			if c, _ := semver.Compare(tag, best.Tag); best.Tag == "" || c > 0 {
				best = Discovered{Tag: tag, URL: l.Href, PublishedAt: e.Updated, Source: "atom"}
			}
		}
	}
	if best.Tag == "" {
		return Discovered{}, errors.New("release feed lists no stable release")
	}
	return best, nil
}

func (d Discoverer) fromRedirect(ctx context.Context, latestURL, tagPrefix string, hosts []string) (Discovered, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return Discovered{}, fmt.Errorf("build latest request: %w", err)
	}
	resp, err := d.client(hosts, false).Do(req)
	if err != nil {
		return Discovered{}, fmt.Errorf("fetch latest release page: %w", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return Discovered{}, fmt.Errorf("latest release page: status %d, expected a redirect", resp.StatusCode)
	}
	loc, err := resp.Location()
	if err != nil {
		return Discovered{}, fmt.Errorf("latest release redirect: %w", err)
	}
	if loc.Scheme != "https" && !d.AllowHTTP {
		return Discovered{}, errors.New("latest release redirect is not https")
	}
	if !hostAllowed(loc.Hostname(), hosts) {
		return Discovered{}, fmt.Errorf("latest release redirect to %s refused: host not allowed", loc.Host)
	}
	_, tag, ok := strings.Cut(loc.Path, tagPathMarker)
	if !ok || !stableProductTag(tag) {
		return Discovered{}, errors.New("latest release redirect names no usable tag")
	}
	return Discovered{Tag: tag, URL: tagPrefix + tag, Source: "redirect"}, nil
}

// stableProductTag accepts v1.2.3 only: no nested module tags and no prereleases.
func stableProductTag(tag string) bool {
	if !ValidTag(tag) || len(tag) < 2 || tag[0] != 'v' || tag[1] < '0' || tag[1] > '9' {
		return false
	}
	if strings.ContainsAny(tag, "/-+") {
		return false
	}
	_, ok := semver.Compare(tag, tag)
	return ok
}
