package dbupgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbcatalogbuild"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
)

// RemoteConfig wires a RemoteCatalog. Zero values pick production defaults.
type RemoteConfig struct {
	Catalog   *Catalog
	DataDir   string
	URL       string
	Source    string
	PublicKey string
	HTTP      *http.Client
	Tags      dbcatalogbuild.TagLister
	Cycles    dbcatalogbuild.CycleSource
	Env       func(string) (string, bool)
	Now       func() time.Time
	Logger    *slog.Logger
}

func (c RemoteConfig) lookupEnv(k string) (string, bool) {
	if c.Env != nil {
		return c.Env(k)
	}
	return os.LookupEnv(k)
}

// RemoteCatalog keeps the embedded Catalog current from the published file,
// a direct registry build, or an on-demand live lookup. It never blocks
// startup and falls back to the embedded data on any failure.
type RemoteCatalog struct {
	cfg RemoteConfig
	mu  sync.Mutex
	st  remoteState
}

// NewRemoteCatalog loads the on-disk cache and applies it. No network I/O.
func NewRemoteCatalog(cfg RemoteConfig) *RemoteCatalog {
	if cfg.Source == "" {
		if v, ok := cfg.lookupEnv(EnvCatalogSource); ok && strings.EqualFold(strings.TrimSpace(v), CatalogSourceDirect) {
			cfg.Source = CatalogSourceDirect
		} else {
			cfg.Source = CatalogSourcePublished
		}
	}
	if cfg.URL == "" {
		cfg.URL = DefaultCatalogURL
		if v, ok := cfg.lookupEnv(EnvCatalogURL); ok && strings.TrimSpace(v) != "" {
			cfg.URL = strings.TrimSpace(v)
		}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	r := &RemoteCatalog{cfg: cfg}
	r.loadState()
	r.reverifyCached()
	r.applyLocked()
	return r
}

func (r *RemoteCatalog) httpClient() *http.Client {
	if r.cfg.HTTP != nil {
		return r.cfg.HTTP
	}
	return &http.Client{
		Timeout: upstreamTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 || req.URL.Host != via[0].URL.Host || req.URL.Scheme != "https" {
				return errors.New("catalog redirect to a different host or scheme refused")
			}
			return nil
		},
	}
}

// reverifyCached drops a cached published copy that no longer verifies.
func (r *RemoteCatalog) reverifyCached() {
	if r.st.PublishedRaw == "" {
		return
	}
	if r.cfg.PublicKey == "" || dbcatalogbuild.Verify(r.cfg.PublicKey, r.st.Signature, []byte(r.st.PublishedRaw)) != nil {
		r.st.PublishedRaw, r.st.Signature, r.st.ETag = "", "", ""
	}
}

func (r *RemoteCatalog) publishedFile() *dbcatalogbuild.File {
	if r.st.PublishedRaw == "" {
		return nil
	}
	f, err := dbcatalogbuild.Parse([]byte(r.st.PublishedRaw))
	if err != nil {
		return nil
	}
	return &f
}

// base returns the data every engine falls back to and where it came from.
func (r *RemoteCatalog) base() (f *dbcatalogbuild.File, origin string, fetched time.Time) {
	now := r.cfg.Now()
	if r.cfg.Source == CatalogSourceDirect {
		if r.st.Direct != nil && now.Sub(r.st.DirectAt) <= staleServeMax {
			return r.st.Direct, CatalogSourceDirect, r.st.DirectAt
		}
		return nil, CatalogSourceEmbedded, time.Time{}
	}
	if pf := r.publishedFile(); pf != nil && now.Sub(r.st.LastChecked) <= staleServeMax {
		return pf, CatalogSourcePublished, r.st.FetchedAt
	}
	return nil, CatalogSourceEmbedded, time.Time{}
}

func (r *RemoteCatalog) engineData(id string) (e dbcatalogbuild.Engine, origin string, generated, fetched time.Time, ok bool) {
	if le := r.st.Live[id]; le != nil {
		return le.Engine, CatalogSourceLive, le.FetchedAt, le.FetchedAt, true
	}
	f, origin, fetched := r.base()
	if f == nil {
		return dbcatalogbuild.Engine{}, CatalogSourceEmbedded, time.Time{}, time.Time{}, false
	}
	gen, _ := time.Parse(time.RFC3339, f.GeneratedAt)
	if gen.IsZero() {
		gen = fetched
	}
	e, ok = f.Engines[id]
	return e, origin, gen, fetched, ok
}

// applyLocked pushes the current remote data into the Catalog.
func (r *RemoteCatalog) applyLocked() {
	if r.cfg.Catalog == nil {
		return
	}
	for _, id := range r.cfg.Catalog.Engines() {
		if e, _, _, _, ok := r.engineData(id); ok {
			r.cfg.Catalog.ApplyRemote(id, e)
		} else {
			r.cfg.Catalog.ApplyRemote(id, dbcatalogbuild.Engine{})
		}
	}
}

func (r *RemoteCatalog) fail(err error) {
	r.st.FailCount++
	r.st.LastError = err.Error()
	wait := backoffBase << min(r.st.FailCount-1, 5)
	var rl *dbcatalogbuild.RateLimitedError
	if errors.As(err, &rl) && rl.RetryAfter > wait {
		wait = rl.RetryAfter
	}
	r.st.NextAttempt = r.cfg.Now().Add(min(wait, backoffMax))
	_ = r.saveStateLocked()
}

// RefreshPublished fetches the signed published file with a conditional GET.
func (r *RemoteCatalog) RefreshPublished(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reason := r.publishedOffReason(); reason != "" {
		return fmt.Errorf("%w: %s", ErrRemoteDisabled, reason)
	}
	u, err := url.Parse(r.cfg.URL)
	if err != nil || u.Scheme != "https" {
		return errors.New("dbupgrade: catalog URL must be https")
	}
	ctx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.URL, nil)
	if err != nil {
		return fmt.Errorf("dbupgrade: build catalog request: %w", err)
	}
	if r.st.ETag != "" && r.publishedFile() != nil {
		req.Header.Set("If-None-Match", r.st.ETag)
	}
	resp, err := r.httpClient().Do(req)
	if err != nil {
		err = fmt.Errorf("dbupgrade: fetch catalog: %w", err)
		r.fail(err)
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	now := r.cfg.Now()
	switch resp.StatusCode {
	case http.StatusNotModified:
		r.st.LastChecked, r.st.FailCount, r.st.LastError = now, 0, ""
		r.st.NextAttempt = now.Add(publishedInterval)
		return r.saveStateLocked()
	case http.StatusTooManyRequests:
		err = &dbcatalogbuild.RateLimitedError{Source: u.Host, RetryAfter: time.Minute}
		r.fail(err)
		return err
	case http.StatusOK:
	default:
		err = fmt.Errorf("dbupgrade: catalog returned %d", resp.StatusCode)
		r.fail(err)
		return err
	}
	body, err := readCapped(resp.Body, maxCatalogBytes)
	if err != nil {
		r.fail(err)
		return err
	}
	if err := r.acceptPublished(ctx, body, resp.Header.Get("ETag"), now); err != nil {
		r.fail(err)
		return err
	}
	return nil
}

func (r *RemoteCatalog) acceptPublished(ctx context.Context, body []byte, etag string, now time.Time) error {
	sigReq, err := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.URL+".sig", nil)
	if err != nil {
		return fmt.Errorf("dbupgrade: build signature request: %w", err)
	}
	sigResp, err := r.httpClient().Do(sigReq)
	if err != nil {
		return fmt.Errorf("dbupgrade: fetch catalog signature: %w", err)
	}
	defer func() { _ = sigResp.Body.Close() }()
	if sigResp.StatusCode != http.StatusOK {
		return fmt.Errorf("dbupgrade: catalog signature returned %d", sigResp.StatusCode)
	}
	sig, err := readCapped(sigResp.Body, maxSigBytes)
	if err != nil {
		return err
	}
	if err := dbcatalogbuild.Verify(r.cfg.PublicKey, string(sig), body); err != nil {
		return fmt.Errorf("dbupgrade: catalog rejected: %w", err)
	}
	f, err := dbcatalogbuild.Parse(body)
	if err != nil {
		return err
	}
	if err := dbcatalogbuild.ValidateAgainst(f, r.publishedFile()); err != nil {
		return fmt.Errorf("dbupgrade: catalog rejected: %w", err)
	}
	r.st.PublishedRaw, r.st.Signature, r.st.ETag = string(body), strings.TrimSpace(string(sig)), etag
	r.st.LastChecked, r.st.FetchedAt = now, now
	r.st.FailCount, r.st.LastError = 0, ""
	r.st.NextAttempt = now.Add(publishedInterval)
	r.st.Live = nil
	r.applyLocked()
	return r.saveStateLocked()
}

func readCapped(rd io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(rd, limit+1))
	if err != nil {
		return nil, fmt.Errorf("dbupgrade: read catalog: %w", err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("dbupgrade: catalog larger than %d bytes", limit)
	}
	return b, nil
}

// Engines lists the engine ids the catalog knows.
func (r *RemoteCatalog) Engines() []string { return r.cfg.Catalog.Engines() }

// acceptsVersion is the control plane's own gate for a remote version.
func acceptsVersion(engine, version string) bool {
	return database.ValidateEngineVersion(engine, version) == nil
}
