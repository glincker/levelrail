package preview

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/diskspace"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/telemetry"
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
	hostMem  func() (total, available int64, err error)
	diskFree func(path string) (int64, error)

	mu      sync.Mutex
	queue   []job
	running bool
	active  map[string]bool
	seen    map[string]string
	baseCtx context.Context

	opMu sync.Mutex
}

type job struct {
	app   string
	image string
	force bool
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
		fs:       NewFileStore(deps.DataDir + "/previews"),
		now:      time.Now,
		shooter:  CDPShooter{},
		hostMem:  telemetry.HostMemoryBytes,
		diskFree: diskspace.Free,
		active:   map[string]bool{},
		seen:     map[string]string{},
		baseCtx:  context.Background(),
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
	m.enqueue(job{app: app, image: image})
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
	if !m.running {
		m.running = true
		go m.drain()
	}
}

func (m *Manager) drain() {
	for {
		m.mu.Lock()
		if len(m.queue) == 0 {
			m.running = false
			m.mu.Unlock()
			return
		}
		j := m.queue[0]
		m.queue = m.queue[1:]
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
	target, err := m.deps.Resolver.Resolve(ctx, j.app, j.image)
	var skip *SkipError
	switch {
	case errors.As(err, &skip):
		if skip.DeploymentID != "" {
			m.commit(ctx, captured{rec: Record{DeploymentID: skip.DeploymentID, App: j.app, Path: settings.Path, Status: StatusSkipped, Reason: skip.Reason, Detail: skip.Detail}})
		}
		return
	case err != nil:
		m.log.Warn("preview: resolve target failed", slog.String("app", j.app), slog.String("error", err.Error()))
		return
	}
	if !j.force {
		if existing, gerr := m.deps.Store.GetPreviewRecord(ctx, target.DeploymentID); gerr == nil && existing != nil {
			return
		}
	}
	m.commit(ctx, m.capture(ctx, settings, target))
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
	if rec.Status != StatusOK {
		if prev, err := m.deps.Store.GetPreviewRecord(ctx, rec.DeploymentID); err == nil && prev != nil && prev.Status == StatusOK {
			m.log.Info("preview: capture not stored, keeping existing thumbnail", slog.String("app", rec.App), slog.String("reason", rec.Reason))
			return
		}
	}
	if rec.Status == StatusOK {
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

	deadline := time.Now().Add(m.cfg.Timeout)
	runCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	browser, err := m.deps.Runner.StartBrowser(runCtx, m.browserSpec(t))
	if err != nil {
		m.log.Warn("preview: start browser failed", slog.String("app", s.App), slog.String("error", err.Error()))
		return fail(StatusFailed, ReasonCaptureFailed, "the capture browser could not start")
	}
	defer browser.Close()

	settle := maxSettle + time.Duration(s.WaitMS)*time.Millisecond
	res, err := m.shooter.Shoot(runCtx, browser.Endpoint(), ShotRequest{
		URL:   appURL(t, s.Path),
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
	rec.HTTPStatus = res.HTTPStatus
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
	rec.Status, rec.Bytes, rec.Width, rec.Height = StatusOK, int64(len(thumb.JPEG)), thumb.Width, thumb.Height
	return captured{rec: rec, jpeg: thumb.JPEG}
}

// appURL addresses the app inside its private Docker network, which is plain
// HTTP by design; path is validated to be an absolute path on the app.
func appURL(t Target, path string) string {
	base := url.URL{Scheme: "http", Host: net.JoinHostPort(t.Host, strconv.Itoa(t.Port))}
	return base.String() + path
}

func (m *Manager) browserSpec(t Target) docker.BrowserSpec {
	suffix := make([]byte, 4)
	_, _ = rand.Read(suffix)
	return docker.BrowserSpec{
		Name:    m.deps.Namespace + "-preview-" + hex.EncodeToString(suffix),
		Image:   m.cfg.Image,
		Command: []string{"--hide-scrollbars", "--user-data-dir=/tmp/profile"},
		Env:     map[string]string{"HOME": "/tmp"},
		Labels: map[string]string{
			m.roleLabel():                            roleValue,
			m.deps.Namespace + deploymentLabelSuffix: t.DeploymentID,
		},
		Network:     t.Network,
		User:        "nobody",
		MemoryBytes: m.cfg.MemoryMB << 20,
		NanoCPUs:    int64(m.cfg.CPUs * 1e9),
		PidsLimit:   capturePidsLimit,
		ShmBytes:    captureShmBytes,
		Tmpfs:       map[string]string{"/tmp": "rw,nosuid,size=" + strconv.Itoa(captureTmpBytes>>20) + "m,mode=1777"},
		DebugPort:   debugPort,
	}
}
