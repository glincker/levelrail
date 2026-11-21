package preview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/kit/diskspace"
)

const (
	roleLabelSuffix       = ".role"
	deploymentLabelSuffix = ".preview-deployment"
	roleValue             = "preview-capture"
	captureShmBytes       = 256 << 20
	capturePidsLimit      = 256
	captureTmpBytes       = 128 << 20
	debugPort             = 9222
	startupReserve        = 8 * time.Second
	maxSettle             = 10 * time.Second
)

// Deps are the collaborators a Manager needs.
type Deps struct {
	Store    Store
	Resolver Resolver
	Runner   Runner
	// DataDir is the platform data directory; thumbnails live in DataDir/previews.
	DataDir string
	// Namespace prefixes the labels on capture containers (the brand short name).
	Namespace string
	Logger    *slog.Logger
}

// Manager captures, stores and prunes deploy preview thumbnails. It holds no
// goroutine while idle: a worker exists only while jobs are queued, plus one
// coarse sweep ticker after Start.
type Manager struct {
	cfg  Config
	deps Deps
	log  *slog.Logger
	fs   *FileStore

	now      func() time.Time
	shooter  Shooter
	fetcher  MetaFetcher
	hostMem  func() (total, available int64, err error)
	diskFree func(path string) (int64, error)

	mu      sync.Mutex
	queue   []job
	workers int
	active  map[string]bool
	seen    map[string]string
	baseCtx context.Context
	// lastRun and stale rate-limit automatic renders per app; both are
	// memory only, so a restart merely allows one more render.
	lastRun map[string]time.Time
	stale   map[string]bool

	liveSrc LiveSource
	pool    *browserPool
	lookup  func(ctx context.Context, host string) ([]netip.Addr, error)

	opMu sync.Mutex
}

type job struct {
	app   string
	image string
	force bool
	// auto marks a render the platform asked for (deploy or first view), which
	// the per-app minimum interval governs; a manual capture ignores it.
	auto bool
}

// Option adjusts a Manager, mainly for tests.
type Option func(*Manager)

// WithClock overrides the time source.
func WithClock(now func() time.Time) Option { return func(m *Manager) { m.now = now } }

// WithHostProbes overrides the free RAM and free disk readings.
func WithHostProbes(mem func() (int64, int64, error), disk func(string) (int64, error)) Option {
	return func(m *Manager) { m.hostMem, m.diskFree = mem, disk }
}

// WithShooter overrides the browser driver.
func WithShooter(s Shooter) Option { return func(m *Manager) { m.shooter = s } }

// New builds a Manager. Nothing runs until a job is queued or Start is called.
func New(cfg Config, deps Deps, opts ...Option) *Manager {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	m := &Manager{
		cfg: cfg, deps: deps, log: deps.Logger,
		fetcher:  newHTTPFetcher(cfg),
		fs:       NewFileStore(deps.DataDir + "/previews"),
		now:      time.Now,
		shooter:  CDPShooter{},
		hostMem:  telemetry.HostMemoryBytes,
		diskFree: diskspace.Free,
		active:   map[string]bool{},
		seen:     map[string]string{},
		lastRun:  map[string]time.Time{},
		stale:    map[string]bool{},
		pool:     newBrowserPool(),
		baseCtx:  context.Background(),
		lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		},
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Config returns the effective configuration.
func (m *Manager) Config() Config { return m.cfg }

func (m *Manager) roleLabel() string { return m.deps.Namespace + roleLabelSuffix }

// NotifyReady is called each time an app's release is observed serving. It
// is cheap after the first call per release (image tag plus the running image
// ID, so a rebuilt mutable tag counts as a new release) and never blocks.
func (m *Manager) NotifyReady(app, image, runningImageID string) {
	if !m.cfg.Enabled {
		return
	}
	release := image + "\x00" + runningImageID
	m.mu.Lock()
	if m.seen[app] == release {
		m.mu.Unlock()
		return
	}
	m.seen[app] = release
	m.mu.Unlock()
	m.clearFail(m.baseCtx, app)
	if !m.intervalOK(app) {
		m.mu.Lock()
		m.stale[app] = true
		m.mu.Unlock()
		return
	}
	m.enqueue(job{app: app, image: image, auto: true})
}

// intervalOK reports whether app may be rendered again without breaking the
// per-app minimum interval (a redeploy storm must not re-render each time).
func (m *Manager) intervalOK(app string) bool {
	gap := m.live(m.baseCtx).MinInterval
	m.mu.Lock()
	defer m.mu.Unlock()
	last, ok := m.lastRun[app]
	return !ok || gap <= 0 || m.now().Sub(last) >= gap
}

// Capture queues a manual recapture of app's current release.
func (m *Manager) Capture(ctx context.Context, app string) error {
	if !m.cfg.Enabled {
		return fmt.Errorf("%w: previews are switched off on this server", ErrDisabled)
	}
	s, err := m.Settings(ctx, app)
	if err != nil {
		return err
	}
	if !s.Enabled {
		return fmt.Errorf("%w: previews are off for this app", ErrDisabled)
	}
	m.enqueue(job{app: app, force: true})
	return nil
}

// Capturing reports whether a job for app is queued or running.
func (m *Manager) Capturing(app string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.active[app] {
		return true
	}
	for _, j := range m.queue {
		if j.app == app {
			return true
		}
	}
	return false
}

func (m *Manager) enqueue(j job) {
	conc := m.live(m.baseCtx).MaxConcurrent
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.queue[:0]
	for _, q := range m.queue {
		if q.app != j.app {
			kept = append(kept, q)
		}
	}
	m.queue = kept
	for len(m.queue) >= m.cfg.QueueDepth {
		m.queue = m.queue[1:]
	}
	m.queue = append(m.queue, j)
	if m.workers < conc {
		m.workers++
		go m.drain()
	}
}

// next pops the first job whose app is not already rendering, so two workers
// never render the same app at once.
func (m *Manager) next() (job, bool) {
	for i, q := range m.queue {
		if !m.active[q.app] {
			m.queue = append(m.queue[:i:i], m.queue[i+1:]...)
			return q, true
		}
	}
	return job{}, false
}

func (m *Manager) drain() {
	for {
		m.mu.Lock()
		j, ok := m.next()
		if !ok {
			m.workers--
			m.mu.Unlock()
			return
		}
		m.active[j.app] = true
		m.mu.Unlock()

		m.process(m.baseCtx, j)

		m.mu.Lock()
		delete(m.active, j.app)
		m.mu.Unlock()
	}
}

func (m *Manager) process(ctx context.Context, j job) {
	defer func() {
		if r := recover(); r != nil {
			m.log.Error("preview: capture panicked", slog.String("app", j.app), slog.Any("panic", r))
		}
	}()
	settings, err := m.Settings(ctx, j.app)
	if err != nil {
		m.log.Error("preview: load settings failed", slog.String("app", j.app), slog.String("error", err.Error()))
		return
	}
	if !m.cfg.Enabled || !settings.Enabled {
		return
	}
	if settings.Mode == ModeMetadata {
		m.processMeta(ctx, settings, j)
		return
	}
	if j.auto && !m.renderable(ctx, j.app) {
		return
	}
	target, err := m.deps.Resolver.Resolve(ctx, j.app, j.image)
	var skip *SkipError
	switch {
	case errors.As(err, &skip):
		if skip.DeploymentID != "" {
			m.setFail(ctx, j.app, skip.Reason)
			m.commit(ctx, captured{rec: Record{DeploymentID: skip.DeploymentID, App: j.app, Path: settings.Path, Status: StatusSkipped, Reason: skip.Reason, Detail: skip.Detail}})
		}
		return
	case err != nil:
		m.log.Warn("preview: resolve target failed", slog.String("app", j.app), slog.String("error", err.Error()))
		return
	}
	if !j.force {
		// A text card or site image from the metadata tier does not count: the
		// owner asked for a real render.
		if existing, gerr := m.deps.Store.GetPreviewRecord(ctx, target.DeploymentID); gerr == nil && existing != nil &&
			(existing.Status != StatusOK || existing.Source == SourceScreenshot) {
			return
		}
	}
	m.mu.Lock()
	m.lastRun[j.app] = m.now()
	delete(m.stale, j.app)
	m.mu.Unlock()
	c := m.capture(ctx, settings, target)
	if c.rec.Status == StatusOK {
		m.clearFail(ctx, j.app)
	} else {
		m.setFail(ctx, j.app, c.rec.Reason)
	}
	m.commit(ctx, c)
}

// renderable is false for apps whose kind has no page worth rendering. A
// failing gate means no render, never a guess.
func (m *Manager) renderable(ctx context.Context, app string) bool {
	g, ok := m.deps.Resolver.(RenderGate)
	if !ok {
		return true
	}
	ok, err := g.Renderable(ctx, app)
	if err != nil {
		m.log.Warn("preview: kind check failed", slog.String("app", app), slog.String("error", err.Error()))
		return false
	}
	return ok
}

// captured is a capture attempt's outcome: a record, plus the thumbnail when
// the record is StatusOK.
type captured struct {
	rec  Record
	jpeg []byte
}

// commit stores a capture outcome unless the app was deleted or opted out
// while the browser ran, and never lets a skip or failure replace a thumbnail
// that already exists. It holds opMu so it cannot interleave with DeleteApp.
func (m *Manager) commit(ctx context.Context, c captured) {
	rec := c.rec
	rec.CapturedAt = m.now().UTC()
	m.opMu.Lock()
	defer m.opMu.Unlock()
	if exists, err := m.deps.Resolver.AppExists(ctx, rec.App); err != nil || !exists {
		return
	}
	if s, err := m.Settings(ctx, rec.App); err != nil || !s.Enabled || !m.cfg.Enabled {
		return
	}
	if prev, err := m.deps.Store.GetPreviewRecord(ctx, rec.DeploymentID); err == nil && prev != nil && keepsPrevious(rec, *prev) {
		m.log.Info("preview: capture not stored, keeping existing preview", slog.String("app", rec.App), slog.String("reason", rec.Reason))
		return
	}
	if rec.HasFile() {
		if err := m.fs.Write(rec.App, rec.DeploymentID, c.jpeg); err != nil {
			m.log.Error("preview: write thumbnail failed", slog.String("app", rec.App), slog.String("error", err.Error()))
			return
		}
	}
	if err := m.deps.Store.UpsertPreviewRecord(ctx, rec); err != nil {
		m.log.Error("preview: save record failed", slog.String("app", rec.App), slog.String("deployment_id", rec.DeploymentID), slog.String("error", err.Error()))
		return
	}
	if rec.Status != StatusOK {
		m.log.Info("preview: capture not stored", slog.String("app", rec.App), slog.String("deployment_id", rec.DeploymentID), slog.String("status", rec.Status), slog.String("reason", rec.Reason))
	}
	if err := m.enforceLocked(ctx); err != nil {
		m.log.Warn("preview: retention pass failed", slog.String("error", err.Error()))
	}
}

// keepsPrevious reports whether next must not replace prev: a skip or failure
// never replaces a successful preview, and a card never replaces an image.
func keepsPrevious(next, prev Record) bool {
	switch {
	case next.Status != StatusOK:
		return prev.Status == StatusOK
	case next.Source == SourceCard:
		return prev.HasFile()
	}
	return false
}

func (m *Manager) gates() (reason, detail string) {
	if _, avail, err := m.hostMem(); err == nil && avail < m.cfg.MinFreeRAM {
		return ReasonLowRAM, fmt.Sprintf("%d MB free, need %d MB", avail>>20, m.cfg.MinFreeRAM>>20)
	}
	if free, err := m.diskFree(m.deps.DataDir); err == nil && free < m.cfg.MinFreeDisk {
		return ReasonLowDisk, fmt.Sprintf("%d MB free, need %d MB", free>>20, m.cfg.MinFreeDisk>>20)
	}
	return "", ""
}

func (m *Manager) capture(ctx context.Context, s AppSettings, t Target) captured {
	rec := Record{DeploymentID: t.DeploymentID, App: s.App, Path: s.Path}
	fail := func(status, reason, detail string) captured {
		rec.Status, rec.Reason, rec.Detail = status, reason, detail
		return captured{rec: rec}
	}
	if reason, detail := m.gates(); reason != "" {
		return fail(StatusSkipped, reason, detail)
	}
	if err := m.ensureImage(ctx); err != nil {
		m.log.Warn("preview: browser image unavailable", slog.String("image", m.cfg.Image), slog.String("error", err.Error()))
		return fail(StatusSkipped, ReasonImageUnusable, "could not pull or find the browser image")
	}

	lv := m.live(ctx)
	deadline := time.Now().Add(lv.RenderTimeout)
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	shotURL, network, extra := appURL(t, s.Path), t.Network, []string(nil)
	if t.PublicURL != "" {
		u, args, perr := m.publicTarget(runCtx, t.PublicURL, s.Path)
		if perr != nil {
			m.log.Warn("preview: public render target refused", slog.String("app", s.App), slog.String("error", perr.Error()))
			return fail(StatusFailed, ReasonBlockedTarget, "the public address is not an allowed render target")
		}
		shotURL, network, extra = u, publicNetwork, args
	}
	browser, release, err := m.acquireBrowser(runCtx, m.browserSpec(t, lv, network, extra), lv.IdleStop)
	if err != nil {
		m.log.Warn("preview: start browser failed", slog.String("app", s.App), slog.String("error", err.Error()))
		return fail(StatusFailed, ReasonCaptureFailed, "the capture browser could not start")
	}
	reuse := false
	defer func() { release(reuse) }()

	settle := maxSettle + time.Duration(s.WaitMS)*time.Millisecond
	res, err := m.shooter.Shoot(runCtx, browser.Endpoint(), ShotRequest{
		URL:   shotURL,
		Width: m.cfg.ViewportW, Height: m.cfg.ViewportH,
		Settle:   min(settle, time.Until(deadline)-startupReserve/2),
		Deadline: deadline,
	})
	if err != nil {
		if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return fail(StatusFailed, ReasonTimeout, "the capture exceeded its time limit")
		}
		m.log.Warn("preview: capture failed", slog.String("app", s.App), slog.String("error", err.Error()))
		return fail(StatusFailed, ReasonCaptureFailed, "the browser could not capture the page")
	}
	reuse = true
	rec.HTTPStatus = res.HTTPStatus
	if t.PublicURL != "" && !sameHostURL(res.FinalURL, shotURL) {
		return fail(StatusSkipped, ReasonRedirect, "the page redirects away from the app")
	}
	if reason, detail := res.Classify(s.Path); reason != "" {
		return fail(StatusSkipped, reason, detail)
	}
	thumb, err := BuildThumb(res.PNG, m.cfg.ThumbWidth, m.cfg.Quality, m.cfg.MaxThumbKB<<10, m.cfg.BlankRatio)
	switch {
	case errors.Is(err, ErrBlankImage):
		return fail(StatusSkipped, ReasonBlankImage, "the page rendered blank")
	case err != nil:
		m.log.Warn("preview: reject capture image", slog.String("app", s.App), slog.String("error", err.Error()))
		return fail(StatusFailed, ReasonBadImage, "the screenshot could not be processed")
	}
	rec.Status, rec.Source, rec.Bytes, rec.Width, rec.Height = StatusOK, SourceScreenshot, int64(len(thumb.JPEG)), thumb.Width, thumb.Height
	return captured{rec: rec, jpeg: thumb.JPEG}
}

// appURL addresses the app inside its private Docker network, which is plain
// HTTP by design; path is validated to be an absolute path on the app.
func appURL(t Target, path string) string {
	base := url.URL{Scheme: "http", Host: net.JoinHostPort(t.Host, strconv.Itoa(t.Port))}
	return base.String() + path
}

func (m *Manager) browserSpec(t Target, lv Live, network string, extra []string) docker.BrowserSpec {
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	return docker.BrowserSpec{
		Name:    m.deps.Namespace + "-preview-" + hex.EncodeToString(suffix),
		Image:   m.cfg.Image,
		Command: append([]string{"--hide-scrollbars", "--user-data-dir=/tmp/profile"}, extra...),
		Env:     map[string]string{"HOME": "/tmp"},
		Labels: map[string]string{
			m.roleLabel():                            roleValue,
			m.deps.Namespace + deploymentLabelSuffix: t.DeploymentID,
		},
		Network:     network,
		User:        "nobody",
		MemoryBytes: lv.MemoryMB << 20,
		NanoCPUs:    int64(lv.CPUs * 1e9),
		PidsLimit:   capturePidsLimit,
		ShmBytes:    captureShmBytes,
		Tmpfs:       map[string]string{"/tmp": "rw,nosuid,size=" + strconv.Itoa(captureTmpBytes>>20) + "m,mode=1777"},
		DebugPort:   debugPort,
	}
}
