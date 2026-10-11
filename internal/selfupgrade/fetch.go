package selfupgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
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
	if _, err := get(ctx, client, root+url.PathEscape(asset), out.BinaryPath, limit, 0o700); err != nil {
		return Fetched{}, fmt.Errorf("download %s %s: %w", asset, tag, err)
	}
	if _, err := get(ctx, client, root+checksumsAsset, out.ChecksumsPath, smallMaxBytes, 0o600); err != nil {
		return Fetched{}, fmt.Errorf("download checksums for %s: %w", tag, err)
	}
	bundle := filepath.Join(dir, bundleAsset)
	status, err := get(ctx, client, root+bundleAsset, bundle, smallMaxBytes, 0o600)
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
		c = &http.Client{Timeout: 10 * time.Minute}
	}
	cp := *c
	cp.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= redirectLimit {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" && !f.AllowHTTP {
			return fmt.Errorf("redirect to %s refused: not https", req.URL.Host)
		}
		return nil
	}
	return &cp
}

func get(ctx context.Context, c *http.Client, u, dest string, limit int64, mode os.FileMode) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("build request: %w", err)
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return resp.StatusCode, fmt.Errorf("status %d", resp.StatusCode)
	}
	tmp := dest + ".part"
	out, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode) //nolint:gosec // dest is under the upgrade's work directory
	if err != nil {
		return resp.StatusCode, fmt.Errorf("create %s: %w", filepath.Base(dest), err)
	}
	n, err := io.Copy(out, io.LimitReader(resp.Body, limit+1))
	closeErr := out.Close()
	switch {
	case err != nil:
		_ = os.Remove(tmp)
		return resp.StatusCode, fmt.Errorf("write %s: %w", filepath.Base(dest), err)
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
