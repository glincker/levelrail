package importplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

// Env vars bounding repository inspection.
const (
	EnvMaxFileBytes = "APP_IMPORT_MAX_FILE_BYTES"
	EnvMaxFiles     = "APP_IMPORT_MAX_FILES"
	EnvFetchTimeout = "APP_IMPORT_FETCH_TIMEOUT"
)

const (
	defaultMaxFileBytes = 512 * 1024
	defaultMaxFiles     = 32
	defaultFetchTimeout = 20 * time.Second
)

// ErrFileLimit is returned once a plan has read its allowed number of files.
var ErrFileLimit = errors.New("importplan: repository file limit reached")

// FileSource reads single files from a repository. ok is false for a missing file.
type FileSource interface {
	ReadFile(ctx context.Context, repoURL, ref, path string) (data []byte, ok bool, err error)
}

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil && v > 0 {
		return v
	}
	return def
}

// HTTPFiles reads raw files from a git host over http(s) through the
// SSRF-guarded client, so nothing is cloned or written to disk.
type HTTPFiles struct {
	Client   *http.Client
	MaxBytes int64
	MaxFiles int

	reads int
}

// NewHTTPFiles builds a FileSource with limits taken from the environment.
func NewHTTPFiles() *HTTPFiles {
	c := netguard.NewClient()
	c.Timeout = defaultFetchTimeout
	if v, err := time.ParseDuration(os.Getenv(EnvFetchTimeout)); err == nil && v > 0 {
		c.Timeout = v
	}
	c.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("importplan: too many redirects")
		}
		return nil
	}
	return &HTTPFiles{Client: c, MaxBytes: int64(envInt(EnvMaxFileBytes, defaultMaxFileBytes)), MaxFiles: envInt(EnvMaxFiles, defaultMaxFiles)}
}

// rawURL maps a repo URL to the host's raw-file endpoint.
func rawURL(repoURL, ref, path string) (string, error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", fmt.Errorf("importplan: parse repo url: %w", err)
	}
	if ref == "" {
		ref = "HEAD"
	}
	host := strings.ToLower(u.Hostname())
	p := strings.TrimSuffix(u.Path, "/")
	switch {
	case host == "github.com":
		return "https://raw.githubusercontent.com" + p + "/" + url.PathEscape(ref) + "/" + path, nil
	case strings.Contains(host, "gitlab"):
		return u.Scheme + "://" + u.Host + p + "/-/raw/" + url.PathEscape(ref) + "/" + path, nil
	case host == "bitbucket.org":
		return u.Scheme + "://" + u.Host + p + "/raw/" + url.PathEscape(ref) + "/" + path, nil
	default:
		return u.Scheme + "://" + u.Host + p + "/raw/" + url.PathEscape(ref) + "/" + path, nil
	}
}

// ReadFile fetches one file, capped at MaxBytes.
func (h *HTTPFiles) ReadFile(ctx context.Context, repoURL, ref, path string) ([]byte, bool, error) {
	if h.reads >= h.MaxFiles {
		return nil, false, ErrFileLimit
	}
	h.reads++
	target, err := rawURL(repoURL, ref, path)
	if err != nil {
		return nil, false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, false, fmt.Errorf("importplan: build request: %w", err)
	}
	resp, err := h.Client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("importplan: fetch %s: %w", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, false, nil
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, false, fmt.Errorf("importplan: repository is private or blocked (HTTP %d); connect it through a git provider instead", resp.StatusCode)
	case resp.StatusCode != http.StatusOK:
		return nil, false, fmt.Errorf("importplan: fetch %s: HTTP %d", path, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, h.MaxBytes+1))
	if err != nil {
		return nil, false, fmt.Errorf("importplan: read %s: %w", path, err)
	}
	if int64(len(data)) > h.MaxBytes {
		return nil, false, fmt.Errorf("importplan: %s is larger than %d bytes", path, h.MaxBytes)
	}
	return data, true, nil
}
