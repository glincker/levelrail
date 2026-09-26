package preview

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

// PageResult is a fetched page's outcome.
type PageResult struct {
	HTTPStatus int
	// Base is the final page URL, on the app's own origin.
	Base *url.URL
	Meta PageMeta
}

// MetaFetcher fetches a page and its image for the metadata tier.
type MetaFetcher interface {
	FetchPage(ctx context.Context, t MetaTarget, path string) (*PageResult, error)
	FetchImage(ctx context.Context, t MetaTarget, base *url.URL, ref string) ([]byte, error)
}

var (
	errOffsiteRedirect = errors.New("preview: redirect leaves the app")
	errTooLarge        = errors.New("preview: response exceeds its size cap")
)

type httpFetcher struct {
	cfg      Config
	external *http.Client
}

func newHTTPFetcher(cfg Config) *httpFetcher {
	c := netguard.NewClient()
	c.Timeout = cfg.MetaTimeout
	c.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		if len(via) > cfg.MetaRedirect {
			return fmt.Errorf("preview: stopped after %d redirects", cfg.MetaRedirect)
		}
		return nil
	}
	return &httpFetcher{cfg: cfg, external: c}
}

func origin(t MetaTarget) string { return net.JoinHostPort(t.Host, strconv.Itoa(t.Port)) }

// internalClient always dials the app's published port, whatever host the
// request names, so it can only ever reach that one app.
func (f *httpFetcher) internalClient(t MetaTarget) *http.Client {
	d := &net.Dialer{Timeout: f.cfg.MetaTimeout}
	return &http.Client{
		Timeout: f.cfg.MetaTimeout,
		Transport: &http.Transport{
			Proxy:             nil,
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return d.DialContext(ctx, network, t.Dial)
			},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > f.cfg.MetaRedirect {
				return fmt.Errorf("preview: stopped after %d redirects", f.cfg.MetaRedirect)
			}
			if !strings.EqualFold(req.URL.Host, origin(t)) {
				return errOffsiteRedirect
			}
			return nil
		},
	}
}

// get requests u with c. A non-empty host overrides the Host header, so an
// app that routes by domain still answers as it would to a visitor.
func (f *httpFetcher) get(ctx context.Context, c *http.Client, u, accept, host string) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, f.cfg.MetaTimeout)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("preview: build request: %w", err)
	}
	req.Host = host
	req.Header.Set("Accept", accept)
	req.Header.Set("User-Agent", "deploy-preview/1")
	resp, err := c.Do(req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("preview: fetch %s: %w", req.URL.Redacted(), err)
	}
	resp.Body = &cancelBody{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error {
	err := b.ReadCloser.Close()
	b.cancel()
	return err
}

// FetchPage GETs path on the app and parses its head. Non-2xx answers return
// the status with a nil Meta and no error so the caller can classify them.
func (f *httpFetcher) FetchPage(ctx context.Context, t MetaTarget, path string) (*PageResult, error) {
	client := f.internalClient(t)
	defer client.CloseIdleConnections()
	pageURL := url.URL{Scheme: "http", Host: origin(t)}
	resp, err := f.get(ctx, client, pageURL.String()+path, "text/html", "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	out := &PageResult{HTTPStatus: resp.StatusCode, Base: resp.Request.URL}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, nil
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); ct != "" && !strings.Contains(ct, "html") {
		return nil, fmt.Errorf("%w: content type %q", ErrNotHTML, ct)
	}
	out.Meta = ParseMeta(io.LimitReader(resp.Body, f.cfg.MetaMaxHTML))
	return out, nil
}

// ErrNotHTML means the page answered with something other than HTML.
var ErrNotHTML = errors.New("preview: page is not HTML")

// FetchImage downloads ref (resolved against base) through the guarded path:
// the app's own origin and its public domains go to the app's port, anything
// else goes through the SSRF-guarded external client.
func (f *httpFetcher) FetchImage(ctx context.Context, t MetaTarget, base *url.URL, ref string) ([]byte, error) {
	u, err := ResolveRef(base, ref)
	if err != nil {
		return nil, err
	}
	client, target, host := f.external, u.String(), ""
	if f.ownedHost(t, u) {
		ic := f.internalClient(t)
		defer ic.CloseIdleConnections()
		internal := *u
		internal.Scheme, internal.Host = "http", origin(t)
		client, target = ic, internal.String()
		if !strings.EqualFold(u.Host, origin(t)) {
			host = u.Host
		}
	}
	resp, err := f.get(ctx, client, target, "image/jpeg,image/png,*/*;q=0.1", host)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("preview: image answered %d", resp.StatusCode)
	}
	if resp.ContentLength > f.cfg.MetaMaxImage {
		return nil, errTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, f.cfg.MetaMaxImage+1))
	if err != nil {
		return nil, fmt.Errorf("preview: read image: %w", err)
	}
	if int64(len(data)) > f.cfg.MetaMaxImage {
		return nil, errTooLarge
	}
	return data, nil
}

func (f *httpFetcher) ownedHost(t MetaTarget, u *url.URL) bool {
	if strings.EqualFold(u.Host, origin(t)) || strings.EqualFold(u.Hostname(), t.Host) && u.Port() == "" {
		return true
	}
	for _, d := range t.Domains {
		if strings.EqualFold(u.Hostname(), d) {
			return true
		}
	}
	return false
}
