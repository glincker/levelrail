package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/sourceupload"
	"github.com/GLINCKER/levelrail/internal/spool"
	"github.com/GLINCKER/levelrail/kit/netguard"
)

// Env names for the Pages "deploy from a URL" download.
const (
	EnvPagesURLMaxMB   = "APP_PAGES_URL_MAX_MB"
	EnvPagesURLTimeout = "APP_PAGES_URL_TIMEOUT"

	defaultPagesURLMaxMB   = 100
	defaultPagesURLTimeout = 60 * time.Second
	pagesURLMaxRedirects   = 3
)

// pagesURLClient is a seam so tests can reach a local TLS server; production
// always uses the SSRF-guarded client, which checks every connection including redirects.
var pagesURLClient = netguard.NewClient

// sourceURLError carries the HTTP status and message for a failed fetch.
type sourceURLError struct {
	status int
	msg    string
}

func (e *sourceURLError) Error() string { return e.msg }

func badSourceURL(msg string) *sourceURLError {
	return &sourceURLError{status: http.StatusBadRequest, msg: msg}
}

// parsePagesSourceURL accepts only https URLs without credentials and refuses
// literal private addresses early; the dial guard still covers DNS answers and redirects.
func parsePagesSourceURL(raw string) (*url.URL, *sourceURLError) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return nil, badSourceURL("source_url must be an https URL")
	}
	if u.User != nil {
		return nil, badSourceURL("source_url must not contain credentials")
	}
	if ip, perr := netip.ParseAddr(u.Hostname()); perr == nil && netguard.IsBlocked(ip) && !netguard.AllowPrivate() {
		return nil, badSourceURL("source_url points at a private or local address")
	}
	return u, nil
}

// downloadPagesSource fetches the archive into a spool file capped by APP_PAGES_URL_MAX_MB.
func (rt *Router) downloadPagesSource(ctx context.Context, u *url.URL) (*os.File, *sourceURLError) {
	limit := envInt64Positive(EnvPagesURLMaxMB, defaultPagesURLMaxMB) << 20
	ctx, cancel := context.WithTimeout(ctx, envDuration(EnvPagesURLTimeout, defaultPagesURLTimeout))
	defer cancel()
	c := pagesURLClient()
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= pagesURLMaxRedirects {
			return errors.New("too many redirects")
		}
		if req.URL.Scheme != "https" {
			return errors.New("redirect left https")
		}
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, badSourceURL("source_url is not a valid URL")
	}
	resp, err := c.Do(req) //nolint:gosec // caller-supplied URL, sent through the SSRF-guarded client
	if err != nil {
		rt.logger.Warn("api: pages source download failed", slog.String("host", u.Hostname()), slog.String("error", err.Error()))
		return nil, badSourceURL("source_url could not be downloaded (unreachable, blocked or timed out)")
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, badSourceURL(fmt.Sprintf("source_url answered HTTP %d", resp.StatusCode))
	}
	if resp.ContentLength > limit {
		return nil, &sourceURLError{http.StatusRequestEntityTooLarge, fmt.Sprintf("source_url is larger than the %d byte limit (%s)", limit, EnvPagesURLMaxMB)}
	}
	f, err := spool.Create("pages-url")
	if errors.Is(err, spool.ErrQuota) {
		return nil, &sourceURLError{http.StatusInsufficientStorage, "upload spool is full, retry shortly"}
	}
	if err != nil {
		return nil, &sourceURLError{http.StatusInternalServerError, "internal error"}
	}
	n, err := io.Copy(f, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		discardSpool(f)
		return nil, badSourceURL("source_url download was interrupted")
	}
	if n > limit {
		discardSpool(f)
		return nil, &sourceURLError{http.StatusRequestEntityTooLarge, fmt.Sprintf("source_url is larger than the %d byte limit (%s)", limit, EnvPagesURLMaxMB)}
	}
	return f, nil
}

func discardSpool(f *os.File) {
	_ = f.Close()
	_ = os.Remove(f.Name())
}

// stagePagesSourceURL downloads, validates and stores the archive for app name
// and returns its upload hash. A zip is converted to the gzip tar the store holds.
func (rt *Router) stagePagesSourceURL(ctx context.Context, name, raw string) (string, *sourceURLError) {
	st := rt.uploadStore()
	if st == nil {
		return "", &sourceURLError{http.StatusNotImplemented, "source upload is not configured on this control plane"}
	}
	u, perr := parsePagesSourceURL(raw)
	if perr != nil {
		return "", perr
	}
	f, derr := rt.downloadPagesSource(ctx, u)
	if derr != nil {
		return "", derr
	}
	defer discardSpool(f)
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], 0); err != nil {
		return "", badSourceURL("source_url is not a .zip or .tar.gz archive")
	}
	limit := envInt64Positive(EnvPagesURLMaxMB, defaultPagesURLMaxMB) << 20
	var body io.Reader = f
	switch {
	case magic[0] == 'P' && magic[1] == 'K':
		size, _ := f.Seek(0, io.SeekEnd)
		tgz, err := spool.Create("pages-url-tgz")
		if err != nil {
			return "", &sourceURLError{http.StatusInternalServerError, "internal error"}
		}
		defer discardSpool(tgz)
		if _, err := sourceupload.ConvertZip(f, size, tgz, uploadLimits()); err != nil {
			return "", archiveError(err)
		}
		if _, err := tgz.Seek(0, io.SeekStart); err != nil {
			return "", &sourceURLError{http.StatusInternalServerError, "internal error"}
		}
		body = tgz
	case magic[0] == 0x1f && magic[1] == 0x8b:
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", &sourceURLError{http.StatusInternalServerError, "internal error"}
		}
	default:
		return "", badSourceURL("source_url is not a .zip or .tar.gz archive")
	}
	meta, _, err := st.Put(name, body, limit)
	if err != nil {
		return "", archiveError(err)
	}
	return meta.Hash, nil
}

func archiveError(err error) *sourceURLError {
	switch {
	case errors.Is(err, sourceupload.ErrTooLarge):
		return &sourceURLError{http.StatusRequestEntityTooLarge, "archive is too large: " + err.Error()}
	case errors.Is(err, sourceupload.ErrUnsafeArchive):
		return badSourceURL("archive rejected: " + err.Error())
	case errors.Is(err, sourceupload.ErrInvalidName):
		return badSourceURL("invalid app name")
	default:
		return badSourceURL("could not read the archive: it must be a .zip or a gzip compressed tar")
	}
}

// deployStaticFromURL starts a static build from a staged upload by calling the
// normal build trigger, so freeze, queue and authz rules apply unchanged.
func (rt *Router) deployStaticFromURL(r *http.Request, name, hash string) (id string, rec *httptest.ResponseRecorder) {
	body, _ := json.Marshal(triggerBuildRequest{UploadHash: hash, Build: triggerBuildBuildInput{Type: "static"}})
	inner := r.Clone(r.Context())
	inner.Body = io.NopCloser(strings.NewReader(string(body)))
	inner.ContentLength = int64(len(body))
	inner.SetPathValue("name", name)
	rec = httptest.NewRecorder()
	rt.handleTriggerBuild(rec, inner)
	var res triggerBuildResponse
	if rec.Code < 300 && json.Unmarshal(rec.Body.Bytes(), &res) == nil {
		id = res.ID
	}
	return id, rec
}
