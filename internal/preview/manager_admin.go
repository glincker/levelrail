package preview

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const (
	stateImageID     = "image_id"
	stateImageRef    = "image_ref"
	stateImageUsedAt = "image_last_used"
	viewedTouchAfter = 10 * time.Minute
)

// Settings returns app's settings, defaulting to off with path "/".
func (m *Manager) Settings(ctx context.Context, app string) (AppSettings, error) {
	s, err := m.deps.Store.GetPreviewSettings(ctx, app)
	if err != nil {
		return AppSettings{}, fmt.Errorf("preview: load settings for %q: %w", app, err)
	}
	s.App = app
	s.Normalize(m.cfg.DefaultMode, s.Mode != "")
	if s.Path == "" {
		s.Path = DefaultPath
	}
	return s, nil
}

// SaveSettings applies patch to app's settings and stores the result.
func (m *Manager) SaveSettings(ctx context.Context, app string, patch SettingsPatch) (AppSettings, error) {
	s, err := m.Settings(ctx, app)
	if err != nil {
		return AppSettings{}, err
	}
	switch {
	case patch.Mode != nil:
		mode, perr := ParseMode(*patch.Mode)
		if perr != nil {
			return AppSettings{}, perr
		}
		s.Mode = mode
	case patch.Enabled != nil && *patch.Enabled:
		s.Mode = ModeScreenshot
	case patch.Enabled != nil:
		s.Mode = ModeOff
	}
	s.Enabled = s.Mode != ModeOff
	if patch.Path != nil {
		s.Path = *patch.Path
	}
	if patch.WaitMS != nil {
		s.WaitMS = *patch.WaitMS
	}
	if err := s.Validate(); err != nil {
		return AppSettings{}, err
	}
	if err := m.deps.Store.SavePreviewSettings(ctx, s); err != nil {
		return AppSettings{}, fmt.Errorf("preview: save settings for %q: %w", app, err)
	}
	return s, nil
}

// List returns app's records, newest first.
func (m *Manager) List(ctx context.Context, app string) ([]Record, error) {
	recs, err := m.deps.Store.ListPreviewRecords(ctx, app)
	if err != nil {
		return nil, fmt.Errorf("preview: list records for %q: %w", app, err)
	}
	return recs, nil
}

// OpenImage returns the file path of an app's stored thumbnail. It marks the
// preview as viewed, at most once per viewedTouchAfter.
func (m *Manager) OpenImage(ctx context.Context, app, deploymentID string) (string, Record, error) {
	rec, err := m.deps.Store.GetPreviewRecord(ctx, deploymentID)
	if err != nil {
		return "", Record{}, fmt.Errorf("preview: load record %q: %w", deploymentID, err)
	}
	if rec == nil || rec.App != app || rec.Status != StatusOK {
		return "", Record{}, ErrNotFound
	}
	p, err := m.fs.Path(app, deploymentID)
	if err != nil {
		return "", Record{}, ErrNotFound
	}
	if !m.fs.Exists(app, deploymentID) {
		return "", Record{}, ErrNotFound
	}
	if now := m.now(); now.Sub(rec.LastViewedAt) > viewedTouchAfter {
		if terr := m.deps.Store.TouchPreviewViewed(ctx, deploymentID, now.UTC()); terr != nil {
			m.log.Warn("preview: mark viewed failed", slog.String("deployment_id", deploymentID), slog.String("error", terr.Error()))
		}
	}
	return p, *rec, nil
}

// DeleteApp removes every preview of a deleted app.
func (m *Manager) DeleteApp(ctx context.Context, app string) error {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	recs, err := m.deps.Store.ListPreviewRecords(ctx, app)
	if err != nil {
		return fmt.Errorf("preview: list records for %q: %w", app, err)
	}
	ids := make([]string, 0, len(recs))
	for _, r := range recs {
		ids = append(ids, r.DeploymentID)
	}
	if err := m.deps.Store.DeletePreviewRecords(ctx, ids); err != nil {
		return fmt.Errorf("preview: delete records for %q: %w", app, err)
	}
	if err := m.deps.Store.DeletePreviewSettings(ctx, app); err != nil {
		return fmt.Errorf("preview: delete settings for %q: %w", app, err)
	}
	m.mu.Lock()
	delete(m.seen, app)
	m.mu.Unlock()
	return m.fs.RemoveApp(app)
}

// PruneResult reports what a prune removed.
type PruneResult struct {
	Removed    int
	FreedBytes int64
}

// Prune applies retention to app now. With all set it removes every preview
// of the app, including the current release's.
func (m *Manager) Prune(ctx context.Context, app string, all bool) (PruneResult, error) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	recs, err := m.deps.Store.ListPreviewRecords(ctx, app)
	if err != nil {
		return PruneResult{}, fmt.Errorf("preview: list records for %q: %w", app, err)
	}
	var victims []Record
	if all {
		victims = recs
	} else {
		protected, perr := m.protected(ctx, recs)
		if perr != nil {
			return PruneResult{}, perr
		}
		for _, e := range PlanEviction(recs, protected, m.now(), m.policy()) {
			victims = append(victims, e.Record)
		}
	}
	res, err := m.deleteRecords(ctx, victims)
	if err != nil {
		return res, err
	}
	if oerr := m.removeOrphanFiles(ctx); oerr != nil {
		m.log.Warn("preview: orphan file sweep failed", slog.String("error", oerr.Error()))
	}
	return res, nil
}

// Usage is the storage a set of previews occupies.
type Usage struct {
	Bytes int64
	Count int
}

// Storage returns app's usage, and the global usage across all apps.
func (m *Manager) Storage(ctx context.Context, app string) (appUsage, total Usage, err error) {
	all, err := m.deps.Store.ListAllPreviewRecords(ctx)
	if err != nil {
		return Usage{}, Usage{}, fmt.Errorf("preview: list records: %w", err)
	}
	for _, r := range all {
		total.Bytes += r.Bytes
		total.Count++
		if r.App == app {
			appUsage.Bytes += r.Bytes
			appUsage.Count++
		}
	}
	return appUsage, total, nil
}

// ImageState is what the platform knows about the browser image it pulled.
type ImageState struct {
	Ref      string
	ID       string
	LastUsed time.Time
}

// BrowserImage returns the tracked browser image, zero when none was pulled by this platform.
func (m *Manager) BrowserImage(ctx context.Context) ImageState {
	id, _ := m.deps.Store.GetPreviewState(ctx, stateImageID)
	if id == "" {
		return ImageState{}
	}
	ref, _ := m.deps.Store.GetPreviewState(ctx, stateImageRef)
	used, _ := m.deps.Store.GetPreviewState(ctx, stateImageUsedAt)
	t, _ := time.Parse(time.RFC3339, used)
	return ImageState{Ref: ref, ID: id, LastUsed: t}
}

func (m *Manager) policy() RetentionPolicy {
	return RetentionPolicy{KeepPerApp: m.cfg.KeepPerApp, TTL: m.cfg.TTL, MaxTotalBytes: m.cfg.MaxTotalBytes}
}

func (m *Manager) protected(ctx context.Context, recs []Record) (map[string]bool, error) {
	out := map[string]bool{}
	seen := map[string]bool{}
	for _, r := range recs {
		if seen[r.App] {
			continue
		}
		seen[r.App] = true
		cur, err := m.deps.Resolver.CurrentDeployment(ctx, r.App)
		if err != nil {
			return nil, fmt.Errorf("preview: current deployment of %q: %w", r.App, err)
		}
		if cur != "" {
			out[cur] = true
		}
	}
	return out, nil
}

func (m *Manager) deleteRecords(ctx context.Context, victims []Record) (PruneResult, error) {
	var res PruneResult
	ids := make([]string, 0, len(victims))
	for _, r := range victims {
		if r.HasFile() {
			if err := m.fs.Remove(r.App, r.DeploymentID); err != nil {
				return res, err
			}
		}
		ids = append(ids, r.DeploymentID)
		res.Removed++
		res.FreedBytes += r.Bytes
	}
	if err := m.deps.Store.DeletePreviewRecords(ctx, ids); err != nil {
		return res, fmt.Errorf("preview: delete records: %w", err)
	}
	return res, nil
}

func (m *Manager) enforceLocked(ctx context.Context) error {
	recs, err := m.deps.Store.ListAllPreviewRecords(ctx)
	if err != nil {
		return fmt.Errorf("preview: list records: %w", err)
	}
	protected, err := m.protected(ctx, recs)
	if err != nil {
		return err
	}
	var victims []Record
	for _, e := range PlanEviction(recs, protected, m.now(), m.policy()) {
		victims = append(victims, e.Record)
	}
	_, err = m.deleteRecords(ctx, victims)
	return err
}

// removeOrphanFiles deletes thumbnail files without an ok record and demotes
// ok records whose file is gone.
func (m *Manager) removeOrphanFiles(ctx context.Context) error {
	files, err := m.fs.List()
	if err != nil {
		return err
	}
	recs, err := m.deps.Store.ListAllPreviewRecords(ctx)
	if err != nil {
		return fmt.Errorf("preview: list records: %w", err)
	}
	okIDs := map[string]bool{}
	for _, r := range recs {
		if r.HasFile() {
			okIDs[r.DeploymentID] = true
		}
	}
	onDisk := map[string]bool{}
	for _, f := range files {
		if !okIDs[f.DeploymentID] {
			if err := m.fs.RemoveEntry(f); err != nil {
				return err
			}
			continue
		}
		onDisk[f.DeploymentID] = true
	}
	var dangling []string
	for _, r := range recs {
		if r.HasFile() && !onDisk[r.DeploymentID] {
			dangling = append(dangling, r.DeploymentID)
		}
	}
	if err := m.deps.Store.DeletePreviewRecords(ctx, dangling); err != nil {
		return fmt.Errorf("preview: delete dangling records: %w", err)
	}
	return nil
}

func (m *Manager) removeDeletedApps(ctx context.Context) error {
	recs, err := m.deps.Store.ListAllPreviewRecords(ctx)
	if err != nil {
		return fmt.Errorf("preview: list records: %w", err)
	}
	checked := map[string]bool{}
	for _, r := range recs {
		if checked[r.App] {
			continue
		}
		checked[r.App] = true
		exists, err := m.deps.Resolver.AppExists(ctx, r.App)
		if err != nil {
			return fmt.Errorf("preview: check app %q: %w", r.App, err)
		}
		if !exists {
			m.dropApp(ctx, r.App)
		}
	}
	return nil
}

func (m *Manager) dropApp(ctx context.Context, app string) {
	recs, err := m.deps.Store.ListPreviewRecords(ctx, app)
	if err != nil {
		m.log.Warn("preview: list records of deleted app failed", slog.String("app", app), slog.String("error", err.Error()))
		return
	}
	if _, err := m.deleteRecords(ctx, recs); err != nil {
		m.log.Warn("preview: delete records of deleted app failed", slog.String("app", app), slog.String("error", err.Error()))
	}
	if err := m.deps.Store.DeletePreviewSettings(ctx, app); err != nil {
		m.log.Warn("preview: delete settings of deleted app failed", slog.String("app", app), slog.String("error", err.Error()))
	}
	if err := m.fs.RemoveApp(app); err != nil {
		m.log.Warn("preview: remove files of deleted app failed", slog.String("app", app), slog.String("error", err.Error()))
	}
}

// Sweep runs every cleanup pass: retention, deleted apps, orphan files,
// orphan capture containers and the idle browser image.
func (m *Manager) Sweep(ctx context.Context) {
	m.opMu.Lock()
	defer m.opMu.Unlock()
	steps := []struct {
		name string
		run  func() error
	}{
		{"deleted apps", func() error { return m.removeDeletedApps(ctx) }},
		{"retention", func() error { return m.enforceLocked(ctx) }},
		{"orphan files", func() error { return m.removeOrphanFiles(ctx) }},
		{"orphan containers", func() error { return m.sweepContainers(ctx) }},
		{"browser image", func() error { return m.pruneImage(ctx) }},
	}
	for _, s := range steps {
		if err := s.run(); err != nil {
			m.log.Warn("preview: sweep step failed", slog.String("step", s.name), slog.String("error", err.Error()))
		}
	}
}

// sweepContainers force-removes capture containers older than twice the
// capture timeout: leftovers of a control plane that died mid-capture.
func (m *Manager) sweepContainers(ctx context.Context) error {
	list, err := m.deps.Runner.ListContainersByLabel(ctx, m.roleLabel()+"="+roleValue)
	if err != nil {
		return fmt.Errorf("preview: list capture containers: %w", err)
	}
	cutoff := m.now().Add(-2 * m.cfg.Timeout)
	var firstErr error
	for _, c := range list {
		if c.Created.After(cutoff) {
			continue
		}
		if err := m.deps.Runner.Remove(ctx, c.ID, true); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("preview: remove orphan container %q: %w", c.Name, err)
		}
	}
	return firstErr
}

// pruneImage removes the browser image this platform pulled once it has been
// idle past ImageTTL (or previews are off), and nothing still uses it. An
// image the platform did not pull is never tracked, so never removed.
func (m *Manager) pruneImage(ctx context.Context) error {
	st := m.BrowserImage(ctx)
	if st.ID == "" {
		return nil
	}
	idle := m.now().Sub(st.LastUsed) > m.cfg.ImageTTL
	if !idle && m.cfg.Enabled {
		return nil
	}
	if m.busy() {
		return nil
	}
	inUse, err := m.deps.Runner.ImageInUse(ctx, st.ID)
	if err != nil {
		return err
	}
	if inUse {
		return nil
	}
	if err := m.deps.Runner.RemoveImageByID(ctx, st.ID); err != nil {
		return err
	}
	for _, k := range []string{stateImageID, stateImageRef, stateImageUsedAt} {
		if err := m.deps.Store.SetPreviewState(ctx, k, ""); err != nil {
			return fmt.Errorf("preview: clear image state: %w", err)
		}
	}
	m.log.Info("preview: removed idle browser image", slog.String("image", st.Ref))
	return nil
}

func (m *Manager) busy() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// ensureImage makes the browser image available, recording it for later
// pruning only when this call pulled it.
func (m *Manager) ensureImage(ctx context.Context) error {
	pullCtx, cancel := context.WithTimeout(ctx, m.cfg.PullTimeout)
	defer cancel()
	id, pulled, err := m.deps.Runner.EnsureImageID(pullCtx, m.cfg.Image)
	if err != nil {
		return err
	}
	tracked := m.BrowserImage(ctx)
	if !pulled && tracked.ID != id {
		return nil
	}
	now := m.now().UTC().Format(time.RFC3339)
	for k, v := range map[string]string{stateImageID: id, stateImageRef: m.cfg.Image, stateImageUsedAt: now} {
		if err := m.deps.Store.SetPreviewState(ctx, k, v); err != nil {
			return fmt.Errorf("preview: record image state: %w", err)
		}
	}
	return nil
}

// Start runs a startup sweep and then one on every SweepInterval until ctx
// ends. It is the only long-lived goroutine.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	m.baseCtx = ctx
	m.mu.Unlock()
	go func() {
		m.Sweep(ctx)
		t := time.NewTicker(m.cfg.SweepInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.Sweep(ctx)
			}
		}
	}()
}

// Summary is the app-level status shown in the UI and CLI.
type Summary struct {
	Settings     AppSettings
	GlobalOn     bool
	Capturing    bool
	App, Total   Usage
	Image        string
	BrowserImage ImageState
	Latest       *Record
}

// Status assembles the app-level summary.
func (m *Manager) Status(ctx context.Context, app string) (Summary, error) {
	s, err := m.Settings(ctx, app)
	if err != nil {
		return Summary{}, err
	}
	appUse, total, err := m.Storage(ctx, app)
	if err != nil {
		return Summary{}, err
	}
	recs, err := m.List(ctx, app)
	if err != nil {
		return Summary{}, err
	}
	sum := Summary{Settings: s, GlobalOn: m.cfg.Enabled, Capturing: m.Capturing(app), App: appUse, Total: total, Image: m.cfg.Image, BrowserImage: m.BrowserImage(ctx)}
	if len(recs) > 0 {
		sum.Latest = &recs[0]
	}
	return sum, nil
}
