package announcements

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// UserAgent is the only identifying header a fetch sends. It carries no
// version, edition or instance id; the request has no query string or cookie.
const UserAgent = "levelrail-announcements"

// DefaultFetchTimeout bounds one feed request.
const DefaultFetchTimeout = 15 * time.Second

// ErrNotHTTPS is returned for a non-https source URL.
var ErrNotHTTPS = errors.New("announcements: feed URL must be https")

// Fetcher does conditional GETs. A nil Client uses the default transport,
// which honours HTTPS_PROXY.
type Fetcher struct {
	Client *http.Client
}

func (f Fetcher) client() *http.Client {
	if f.Client != nil {
		return f.Client
	}
	return &http.Client{
		Timeout: DefaultFetchTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
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
	if u.Scheme != "https" {
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
	resp, err := f.client().Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("announcements: fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified {
		return Result{NotModified: true, ETag: etag}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("announcements: fetch: status %d", resp.StatusCode)
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
