package plugin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

const (
	fetchTimeout     = 15 * time.Second
	maxSignatureSize = 2 << 10
)

// ErrSourceNotFound marks a source URL that answered 404.
var ErrSourceNotFound = errors.New("plugin: source not found")

// FetchFunc downloads at most max bytes from an https URL.
type FetchFunc func(ctx context.Context, rawURL string, limit int64) ([]byte, error)

// Fetch downloads a package file through the SSRF-guarded client. Only https is accepted.
func Fetch(ctx context.Context, rawURL string, limit int64) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return nil, invalid("the source must be an https URL")
	}
	c := netguard.NewClient()
	c.Timeout = fetchTimeout
	c.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("too many redirects")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, invalid("bad source URL: %v", err)
	}
	resp, err := c.Do(req) //nolint:gosec // operator-supplied URL, sent through the SSRF-guarded client
	if err != nil {
		return nil, invalid("fetch %s: %v", u.Host, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrSourceNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, invalid("fetch %s: HTTP %d", u.Host, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("plugin: read %s: %w", u.Host, err)
	}
	if int64(len(body)) > limit {
		return nil, invalid("the file at %s is larger than %d bytes", u.Host, limit)
	}
	return body, nil
}

// LoadSource turns a manifest URL (and optional signature URL, default
// <manifest>.sig) into manifest and signature text. A missing .sig file is not
// an error: the package is then treated as unsigned.
func LoadSource(ctx context.Context, fetch FetchFunc, manifestURL, signatureURL string) (manifest, signature string, err error) {
	raw, err := fetch(ctx, manifestURL, maxManifestBytes)
	if err != nil {
		return "", "", err
	}
	explicit := strings.TrimSpace(signatureURL) != ""
	if !explicit {
		signatureURL = strings.TrimSpace(manifestURL) + ".sig"
	}
	sig, err := fetch(ctx, signatureURL, maxSignatureSize)
	if errors.Is(err, ErrSourceNotFound) && !explicit {
		return string(raw), "", nil
	}
	if err != nil {
		return "", "", err
	}
	return string(raw), strings.TrimSpace(string(sig)), nil
}
