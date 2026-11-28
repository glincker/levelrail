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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/sourceupload"
	"github.com/glincker/levelrail/internal/spool"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/kit/netguard"
)

// Env names for the Pages "deploy from a URL" download.
const (
	EnvPagesURLMaxMB   = "APP_PAGES_URL_MAX_MB"
	EnvPagesURLTimeout = "APP_PAGES_URL_TIMEOUT"
	// EnvPagesURLAllowPrivate lets the Pages fetch reach private addresses; it is
	// separate from the notification opt-in so neither widens the other.
	EnvPagesURLAllowPrivate = "APP_PAGES_URL_ALLOW_PRIVATE"
	EnvPagesURLConcurrency  = "APP_PAGES_URL_CONCURRENCY"
	EnvPagesURLStoreMaxMB   = "APP_PAGES_URL_STORE_MAX_MB"

	defaultPagesURLMaxMB      = 100
	defaultPagesURLTimeout    = 60 * time.Second
	defaultPagesURLConcurrent = 4
	defaultPagesURLStoreMaxMB = 5120
	pagesURLMaxRedirects      = 3

	pagesURLFetchFailed = "source_url could not be downloaded"
)

// pagesURLClient is a seam so tests can reach a local TLS server; production
// uses a guarded client keyed to the Pages env only, checked on every connection including redirects.
var pagesURLClient = func() *http.Client { return netguard.NewClientAllowing(EnvPagesURLAllowPrivate) }

var (
	pagesURLSemOnce sync.Once
	pagesURLSem     chan struct{}
)

func pagesURLSlots() chan struct{} {
	pagesURLSemOnce.Do(func() { pagesURLSem = make(chan struct{}, envInt(EnvPagesURLConcurrency, defaultPagesURLConcurrent)) })
	return pagesURLSem
}

func pagesURLAllowPrivate() bool {
	v, err := strconv.ParseBool(os.Getenv(EnvPagesURLAllowPrivate))
	return err == nil && v
}

// fetchErrDetail reduces a client error to its inner cause so a signed query string never reaches the logs.
func fetchErrDetail(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err.Error()
	}
	return "request failed"
}

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
	if ip, perr := netip.ParseAddr(u.Hostname()); perr == nil && netguard.IsBlocked(ip) && !pagesURLAllowPrivate() {
		return nil, badSourceURL(pagesURLFetchFailed)
	}
	return u, nil
}

// downloadPagesSource fetches the archive into a spool file capped by APP_PAGES_URL_MAX_MB.
func (rt *Router) downloadPagesSource(ctx context.Context, u *url.URL) (*os.File, *sourceURLError) {
	limit := envInt64Positive(EnvPagesURLMaxMB, defaultPagesURLMaxMB) << 20
	if used, uerr := rt.uploadStore().Usage(); uerr == nil && used+limit > envInt64Positive(EnvPagesURLStoreMaxMB, defaultPagesURLStoreMaxMB)<<20 {
		return nil, &sourceURLError{http.StatusInsufficientStorage, "source storage is full, retry later"}
	}
	select {
	case pagesURLSlots() <- struct{}{}:
		defer func() { <-pagesURLSlots() }()
	case <-ctx.Done():
		return nil, badSourceURL(pagesURLFetchFailed)
	}
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
		rt.logger.Warn("api: pages source download failed", slog.String("host", u.Hostname()), slog.String("error", fetchErrDetail(err)))
		return nil, badSourceURL(pagesURLFetchFailed)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		rt.logger.Warn("api: pages source download refused", slog.String("host", u.Hostname()), slog.Int("status", resp.StatusCode))
		return nil, badSourceURL(pagesURLFetchFailed)
	}
	if resp.ContentLength > limit {
		return nil, &sourceURLError{http.StatusRequestEntityTooLarge, fmt.Sprintf("source_url is larger than the %d byte limit (%s)", limit, EnvPagesURLMaxMB)}
	}
	f, err := spool.Create("pages-url")
	if err != nil {
		return nil, spoolError(err)
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

func spoolError(err error) *sourceURLError {
	if errors.Is(err, spool.ErrQuota) {
		return &sourceURLError{http.StatusInsufficientStorage, "upload spool is full, retry shortly"}
	}
	return &sourceURLError{http.StatusInternalServerError, "internal error"}
}

func discardSpool(f *os.File) {
	_ = f.Close()
	_ = os.Remove(f.Name())
}

// stagePagesSourceURL downloads, validates and stores the archive for app name
// and returns its upload hash; fresh is false when the same archive was already stored. A zip is converted to the gzip tar the store holds.
func (rt *Router) stagePagesSourceURL(ctx context.Context, name, raw string) (hash string, fresh bool, serr *sourceURLError) {
	st := rt.uploadStore()
	if st == nil {
		return "", false, &sourceURLError{http.StatusNotImplemented, "source upload is not configured on this control plane"}
	}
	u, perr := parsePagesSourceURL(raw)
	if perr != nil {
		return "", false, perr
	}
	f, derr := rt.downloadPagesSource(ctx, u)
	if derr != nil {
		return "", false, derr
	}
	defer discardSpool(f)
	var magic [4]byte
	if _, err := f.ReadAt(magic[:], 0); err != nil {
		return "", false, badSourceURL("source_url is not a .zip or .tar.gz archive")
	}
	limit := envInt64Positive(EnvPagesURLMaxMB, defaultPagesURLMaxMB) << 20
	var body io.Reader = f
	switch {
	case magic[0] == 'P' && magic[1] == 'K':
		size, _ := f.Seek(0, io.SeekEnd)
		tgz, err := spool.Create("pages-url-tgz")
		if err != nil {
			return "", false, spoolError(err)
		}
		defer discardSpool(tgz)
		if _, err := sourceupload.ConvertZip(f, size, tgz, uploadLimits()); err != nil {
			return "", false, archiveError(err)
		}
		if _, err := tgz.Seek(0, io.SeekStart); err != nil {
			return "", false, &sourceURLError{http.StatusInternalServerError, "internal error"}
		}
		body = tgz
	case magic[0] == 0x1f && magic[1] == 0x8b:
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", false, &sourceURLError{http.StatusInternalServerError, "internal error"}
		}
	default:
		return "", false, badSourceURL("source_url is not a .zip or .tar.gz archive")
	}
	meta, deduped, err := st.Put(name, body, limit)
	if err != nil {
		return "", false, archiveError(err)
	}
	return meta.Hash, !deduped, nil
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

// removeStagedPagesSource drops an archive a failed request staged so it cannot linger.
func (rt *Router) removeStagedPagesSource(name, hash string) {
	if st := rt.uploadStore(); st != nil && hash != "" {
		_ = st.Remove(name, hash)
	}
}

// AuditActionPagesURLDeploy records a deploy from an archive URL by host and path only.
const AuditActionPagesURLDeploy = "pages.deploy.source_url"

func (rt *Router) auditPagesURLDeploy(r *http.Request, app, raw string) {
	u, perr := parsePagesSourceURL(raw)
	if rt.auditLog == nil || perr != nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	detail, _ := json.Marshal(map[string]string{"host": u.Hostname(), "path": u.Path})
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityDeploy, Method: http.MethodPost, Path: "/api/v1/apps/" + app + "/builds", StatusCode: http.StatusAccepted,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action:     AuditActionPagesURLDeploy, Detail: string(detail),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: pages url audit save failed", slog.String("app", app))
	}
}

// isPagesSiteApp tells a Pages site from any other app, so a URL deploy never turns one into the other.
func (rt *Router) isPagesSiteApp(ctx context.Context, name, image string) bool {
	if strings.HasSuffix(image, staticSitePendingTag) {
		return true
	}
	if g, ok := rt.staticSites.(staticSiteGetter); ok {
		if s, err := g.GetStaticSite(ctx, name); err == nil && s != nil {
			return true
		}
	}
	return false
}
