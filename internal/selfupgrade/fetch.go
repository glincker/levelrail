package selfupgrade

import (
	"context"
	"errors"
	"fmt"
	"github.com/GLINCKER/levelrail/internal/netclient"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

const (
	checksumsAsset  = "checksums.txt"
	bundleAsset     = "checksums.txt.sigstore.json"
	defaultMaxBytes = 400 << 20
	smallMaxBytes   = 4 << 20
	redirectLimit   = 5
)

// Fetched is a downloaded release: the binary, its checksums file and the
// signature bundle (empty when the release publishes none).
type Fetched struct {
	BinaryPath    string
	ChecksumsPath string
	BundlePath    string
}

// Fetcher downloads release assets over HTTPS.
type Fetcher struct {
	// BaseURL is the release download prefix with the tag as its last path
	// segment appended by Fetch, for example https://host/org/repo/releases/download.
	BaseURL string
	Client  *http.Client
	// AllowHTTP permits a plain-http BaseURL, for tests against a local server.
	AllowHTTP bool
	MaxBytes  int64
	// RedirectHosts, when set, limits redirects to these hosts and their
	// subdomains, so a release asset cannot bounce the server to an internal address.
	RedirectHosts []string
	// Stall fails a transfer that receives no bytes for this long. Zero uses
	// the default limit.
	Stall time.Duration
	// Deadline caps the whole fetch. Zero uses the default limit.
	Deadline time.Duration
}

// ErrStalled is returned when a download stops receiving data.
var ErrStalled = errors.New("download stalled")

// Progress is a download's byte counters; Total is 0 when the server did not say.
type Progress struct {
	Bytes int64
	Total int64
}

type progressKey struct{}

// WithProgress makes Fetch report the main asset's progress to fn.
func WithProgress(ctx context.Context, fn func(Progress)) context.Context {
	return context.WithValue(ctx, progressKey{}, fn)
}

// Fetch downloads asset, checksums.txt and the signature bundle of tag into
// dir. The checksums file is required, the bundle is optional.
func (f Fetcher) Fetch(ctx context.Context, tag, asset, dir string) (Fetched, error) {
	if !ValidTag(tag) {
		return Fetched{}, fmt.Errorf("refusing unsafe release tag %q", tag)
	}
	if strings.ContainsAny(asset, "/\\") || asset == "" {
		return Fetched{}, fmt.Errorf("refusing unsafe asset name %q", asset)
	}
	base, err := url.Parse(f.BaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "https" && (!f.AllowHTTP || base.Scheme != "http")) {
		return Fetched{}, errors.New("release base URL must be https")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Fetched{}, fmt.Errorf("create download directory: %w", err)
	}
	lim := DefaultLimits()
	if f.Deadline <= 0 {
		f.Deadline = lim.Deadline
	}
	if f.Stall <= 0 {
		f.Stall = lim.Stall
	}
	ctx, cancel := context.WithTimeout(ctx, f.Deadline)
	defer cancel()
	client := f.client()
	limit := f.MaxBytes
	if limit <= 0 {
		limit = defaultMaxBytes
	}
	root := strings.TrimRight(f.BaseURL, "/") + "/" + url.PathEscape(tag) + "/"
	out := Fetched{
		BinaryPath:    filepath.Join(dir, asset),
		ChecksumsPath: filepath.Join(dir, checksumsAsset),
	}
	report, _ := ctx.Value(progressKey{}).(func(Progress))
	if _, err := get(ctx, client, root+url.PathEscape(asset), out.BinaryPath, limit, 0o700, f.Stall, report); err != nil {
		return Fetched{}, fmt.Errorf("download %s %s: %w", asset, tag, deadlineNote(err, f.Deadline))
	}
	if _, err := get(ctx, client, root+checksumsAsset, out.ChecksumsPath, smallMaxBytes, 0o600, f.Stall, nil); err != nil {
		return Fetched{}, fmt.Errorf("download checksums for %s: %w", tag, err)
	}
	bundle := filepath.Join(dir, bundleAsset)
	status, err := get(ctx, client, root+bundleAsset, bundle, smallMaxBytes, 0o600, f.Stall, nil)
	switch {
	case err == nil:
		out.BundlePath = bundle
	case status == http.StatusNotFound:
	default:
		return Fetched{}, fmt.Errorf("download signature bundle for %s: %w", tag, err)
	}
	return out, nil
}

func (f Fetcher) client() *http.Client {
	c := f.Client
	if c == nil {
		c = netclient.New()
	}
	cp := *c
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= redirectLimit {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" && !f.AllowHTTP {
			return fmt.Errorf("redirect to %s refused: not https", req.URL.Host)
		}
		if len(f.RedirectHosts) > 0 && !hostAllowed(req.URL.Hostname(), f.RedirectHosts) {
			return fmt.Errorf("redirect to %s refused: host not allowed", req.URL.Host)
		}
		return nil
	}
	return &cp
}

func hostAllowed(host string, allowed []string) bool {
	host = strings.ToLower(host)
	for _, a := range allowed {
		if host == a || strings.HasSuffix(host, "."+a) {
			return true
		}
	}
	return false
}

func get(ctx context.Context, c *http.Client, u, dest string, limit int64, mode os.FileMode, stall time.Duration, report func(Progress)) (int, error) {
	// The watchdog cancels the request when no byte arrives for stall, which
	// covers a server that never answers as well as one that stops mid-body.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	var last atomic.Int64
	last.Store(time.Now().UnixNano())
	done := make(chan struct{})
	defer close(done)
	go watchStall(done, &last, stall, cancel)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, causeOr(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("status %d", resp.StatusCode)
	}
	last.Store(time.Now().UnixNano())
	tmp := dest + ".part"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) //nolint:gosec // dest is under the upgrade's work directory
	if err != nil {
		return resp.StatusCode, fmt.Errorf("create %s: %w", filepath.Base(dest), err)
	}
	body := &countingReader{r: io.LimitReader(resp.Body, limit+1), last: &last, total: resp.ContentLength, report: report}
	n, err := io.Copy(out, body)
	closeErr := out.Close()
	switch {
	case err != nil:
		_ = os.Remove(tmp)
		return resp.StatusCode, fmt.Errorf("write %s: %w", filepath.Base(dest), causeOr(ctx, err))
	case n > limit:
		_ = os.Remove(tmp)
		return resp.StatusCode, fmt.Errorf("%s exceeds %d bytes", filepath.Base(dest), limit)
	case closeErr != nil:
		_ = os.Remove(tmp)
		return resp.StatusCode, fmt.Errorf("close %s: %w", filepath.Base(dest), closeErr)
	}
	if err := os.Rename(tmp, dest); err != nil {
		return resp.StatusCode, fmt.Errorf("finalize %s: %w", filepath.Base(dest), err)
	}
	return resp.StatusCode, nil
}

func deadlineNote(err error, d time.Duration) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("exceeded the overall time limit of %s: %w", d, err)
	}
	return err
}

// causeOr prefers the cancellation cause (stall, operator cancel) over the
// generic "context canceled" the transport reports.
func causeOr(ctx context.Context, err error) error {
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause
	}
	return err
}

func watchStall(done <-chan struct{}, last *atomic.Int64, stall time.Duration, cancel context.CancelCauseFunc) {
	t := time.NewTicker(max(stall/4, time.Millisecond))
	defer t.Stop()
	for {
		select {
		case <-done:
			return
		case <-t.C:
			if time.Since(time.Unix(0, last.Load())) > stall {
				cancel(fmt.Errorf("%w: no data received for %s", ErrStalled, stall.Round(time.Millisecond)))
				return
			}
		}
	}
}

type countingReader struct {
	r      io.Reader
	last   *atomic.Int64
	total  int64
	n      int64
	report func(Progress)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.n += int64(n)
		c.last.Store(time.Now().UnixNano())
		if c.report != nil {
			c.report(Progress{Bytes: c.n, Total: max(c.total, 0)})
		}
	}
	return n, err
}

// ValidTag accepts release tags such as v1.2.3 or v1.2.3-beta.4 and nothing
// that could escape a URL path or a file name.
func ValidTag(tag string) bool {
	if len(tag) < 2 || len(tag) > 64 || tag[0] != 'v' {
		return false
	}
	for _, r := range tag[1:] {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '.', r == '-', r == '+':
		default:
			return false
		}
	}
	return true
}
