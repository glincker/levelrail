package dbcatalogbuild

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/registrycatalog"
)

const (
	maxEOLBody     = 1 << 20
	upstreamTimout = 10 * time.Second
)

// RateLimitedError is an upstream 429 with its Retry-After.
type RateLimitedError struct {
	Source     string
	RetryAfter time.Duration
}

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("dbcatalogbuild: %s rate limited, retry after %s", e.Source, e.RetryAfter)
}

// Cycle is one endoflife.date release cycle.
type Cycle struct {
	Cycle             string          `json:"cycle"`
	ReleaseDate       string          `json:"releaseDate"`
	EOL               json.RawMessage `json:"eol"`
	Latest            string          `json:"latest"`
	LatestReleaseDate string          `json:"latestReleaseDate"`
	LTS               bool            `json:"lts"`
}

// eolDate is the cycle's end-of-life date, "" when none is announced.
func (c Cycle) eolDate() string {
	var s string
	if err := json.Unmarshal(c.EOL, &s); err != nil {
		return ""
	}
	return s
}

// EOLClient reads cycles from the endoflife.date JSON API.
type EOLClient struct {
	HTTP *http.Client
	Base string
}

// Cycles fetches <base>/<slug>.json.
func (c EOLClient) Cycles(ctx context.Context, slug string) ([]Cycle, error) {
	base := c.Base
	if base == "" {
		base = EOLBase
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: upstreamTimout}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/"+url.PathEscape(slug)+".json", nil)
	if err != nil {
		return nil, fmt.Errorf("dbcatalogbuild: build lifecycle request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("dbcatalogbuild: fetch lifecycle %s: %w", slug, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &RateLimitedError{Source: "endoflife.date", RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dbcatalogbuild: lifecycle %s returned %d", slug, resp.StatusCode)
	}
	var out []Cycle
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxEOLBody)).Decode(&out); err != nil {
		return nil, fmt.Errorf("dbcatalogbuild: decode lifecycle %s: %w", slug, err)
	}
	return out, nil
}

func retryAfter(h string) time.Duration {
	if n, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && n > 0 {
		return time.Duration(n) * time.Second
	}
	return time.Minute
}

func asRateLimit(err error) (time.Duration, bool) {
	var rl *registrycatalog.RateLimitError
	if errors.As(err, &rl) {
		return rl.RetryAfter, true
	}
	return 0, false
}

// RegistryTags lists tags through the OCI distribution API.
type RegistryTags struct {
	Client  *registrycatalog.Client
	BaseURL string
}

// ListTags implements TagLister, mapping a registry 429 to RateLimitedError.
func (r RegistryTags) ListTags(ctx context.Context, repository string) ([]string, error) {
	c := r.Client
	if c == nil {
		c = registrycatalog.NewClient()
	}
	base := r.BaseURL
	if base == "" {
		base = DockerHubBase
	}
	tags, err := c.ListTagsAnonymous(ctx, base, repository)
	if rl, ok := asRateLimit(err); ok {
		return nil, &RateLimitedError{Source: base, RetryAfter: rl}
	}
	return tags, err
}
