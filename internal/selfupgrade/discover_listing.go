package selfupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/semver"
)

// Channel names a listing can serve, matching the update channels.
const (
	// ListingFile is the small document a release mirror publishes next to its release folders.
	ListingFile     = "latest.json"
	listingMaxBytes = 64 << 10
	listingTimeout  = 10 * time.Second
	// ListingChannelStable is the final-release channel.
	ListingChannelStable = "stable"
	// ListingChannelBeta is the prerelease channel.
	ListingChannelBeta = "beta"
	// ListingChannelEdge is the moving image channel.
	ListingChannelEdge = "edge"
)

var edgeListingTag = regexp.MustCompile(`^main-[0-9a-f]{7,40}$`)

// listingEntry names one release. The publish time is optional.
type listingEntry struct {
	Tag         string `json:"tag"`
	PublishedAt string `json:"published_at"`
}

// listing is the latest.json layout: the stable release at the top level,
// plus optional beta and edge entries.
type listing struct {
	listingEntry
	Beta *listingEntry `json:"beta"`
	Edge *listingEntry `json:"edge"`
}

// ParseListing picks the channel's tag out of a mirror listing and applies the
// same tag rules as the public release pages. It never trusts the text beyond the tag shape.
func ParseListing(raw []byte, channel string) (Discovered, error) {
	var l listing
	if err := json.Unmarshal(raw, &l); err != nil {
		return Discovered{}, fmt.Errorf("decode mirror listing: %w", err)
	}
	var e listingEntry
	switch channel {
	case ListingChannelBeta:
		if l.Beta == nil {
			return Discovered{}, errors.New("mirror listing names no beta release")
		}
		e = *l.Beta
		if !betaListingTag(e.Tag) {
			return Discovered{}, errors.New("mirror listing beta tag is not a prerelease tag")
		}
	case ListingChannelEdge:
		if l.Edge == nil || !edgeListingTag.MatchString(l.Edge.Tag) {
			return Discovered{}, errors.New("mirror listing names no edge build")
		}
		e = *l.Edge
	default:
		e = l.listingEntry
		if !stableProductTag(e.Tag) {
			return Discovered{}, errors.New("mirror listing names no stable release")
		}
	}
	if e.PublishedAt != "" {
		if _, err := time.Parse(time.RFC3339, e.PublishedAt); err != nil {
			e.PublishedAt = ""
		}
	}
	return Discovered{Tag: e.Tag, PublishedAt: e.PublishedAt, Source: "mirror"}, nil
}

// betaListingTag accepts v1.2.3-beta.4 style tags only.
func betaListingTag(tag string) bool {
	if !ValidTag(tag) || len(tag) < 2 || tag[0] != 'v' || tag[1] < '0' || tag[1] > '9' || strings.ContainsAny(tag, "/+") || !strings.Contains(tag, "-") {
		return false
	}
	_, ok := semver.Compare(tag, tag)
	return ok
}

// FromMirror reads <base>/latest.json over https, with a size and time cap,
// and returns the channel's newest tag. Downloading and verifying it is unchanged.
func (d Discoverer) FromMirror(ctx context.Context, base, channel string) (Discovered, error) {
	bu, err := url.Parse(strings.TrimSpace(base))
	if err != nil || bu.Host == "" || bu.User != nil || (bu.Scheme != "https" && (!d.AllowHTTP || bu.Scheme != "http")) {
		return Discovered{}, errors.New("mirror base URL must be https")
	}
	timeout := d.Timeout
	if timeout <= 0 {
		timeout = listingTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(bu.String(), "/")+"/"+ListingFile, nil)
	if err != nil {
		return Discovered{}, fmt.Errorf("build mirror listing request: %w", err)
	}
	resp, err := d.client([]string{strings.ToLower(bu.Hostname())}, true).Do(req)
	if err != nil {
		return Discovered{}, fmt.Errorf("fetch mirror listing: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Discovered{}, fmt.Errorf("mirror listing: status %d", resp.StatusCode)
	}
	limit := d.MaxBytes
	if limit <= 0 || limit > listingMaxBytes {
		limit = listingMaxBytes
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return Discovered{}, fmt.Errorf("read mirror listing: %w", err)
	}
	if int64(len(raw)) > limit {
		return Discovered{}, fmt.Errorf("mirror listing exceeds %d bytes", limit)
	}
	found, err := ParseListing(raw, channel)
	if err != nil {
		return Discovered{}, err
	}
	found.URL = strings.TrimRight(bu.String(), "/") + "/" + found.Tag
	return found, nil
}
