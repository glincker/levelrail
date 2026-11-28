package announcements

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// UserAgent is the only identifying header a fetch sends. It carries no
// version, edition or instance id; the request has no query string or cookie.
const UserAgent = "levelrail-announcements"

// DefaultFetchTimeout bounds one feed request.
const DefaultFetchTimeout = 15 * time.Second

// ErrNotHTTPS is returned for a non-https source URL.
var ErrNotHTTPS = errors.New("announcements: feed URL must be https")

// ErrFetch and ErrFetchStatus are the stable failure classes of a feed request;
// their text never carries a host, address or port.
var (
	ErrFetch       = errors.New("announcements: feed request failed")
	ErrFetchStatus = errors.New("announcements: feed server answered with an error status")
)

// Fetcher does conditional GETs. A nil Client uses the guarded client, which
// refuses private, loopback and link-local addresses at connect time.
type Fetcher struct {
	Client *http.Client
	// LoopbackHTTP allows plain http to a loopback host, for dev instances only.
	LoopbackHTTP bool
}

func isLoopbackHost(h string) bool {
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

// client uses the same dial guard as template sources. Only a dev instance
// fetching plain http from loopback gets an unguarded client.
func (f Fetcher) client(devLoopback bool) *http.Client {
	if f.Client != nil {
		return f.Client
	}
	c := netguard.NewClient()
	if devLoopback {
		c = &http.Client{}
	}
	c.Timeout = DefaultFetchTimeout
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return c
}

// Result is one response. NotModified means the ETag still matches.
type Result struct {
	Body        []byte
	ETag        string
	NotModified bool
}

// Get fetches rawURL with If-None-Match when etag is set. The body is capped
// at limit bytes before any parsing; redirects are not followed.
func (f Fetcher) Get(ctx context.Context, rawURL, etag string, limit int64) (Result, error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return Result{}, fmt.Errorf("announcements: bad feed URL")
	}
	devLoopback := f.LoopbackHTTP && u.Scheme == "http" && isLoopbackHost(u.Hostname())
	if u.Scheme != "https" && !devLoopback {
		return Result{}, ErrNotHTTPS
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{}, fmt.Errorf("announcements: build request: %w", err)
	}
	req.Header.Set("User-Agent", UserAgent)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, err := f.client(devLoopback).Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %s", ErrFetch, err.Error())
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified {
		return Result{NotModified: true, ETag: etag}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("%w: status %d", ErrFetchStatus, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return Result{}, fmt.Errorf("announcements: read body: %w", err)
	}
	if int64(len(body)) > limit {
		return Result{}, ErrTooLarge
	}
	return Result{Body: body, ETag: resp.Header.Get("ETag")}, nil
}
