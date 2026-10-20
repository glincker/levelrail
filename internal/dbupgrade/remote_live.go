package dbupgrade

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbcatalogbuild"
	"github.com/GLINCKER/levelrail/internal/registrycatalog"
)

func (r *RemoteCatalog) builder() *dbcatalogbuild.Builder {
	tags := r.cfg.Tags
	if tags == nil {
		tags = dbcatalogbuild.RegistryTags{Client: registrycatalog.NewClient()}
	}
	cycles := r.cfg.Cycles
	if cycles == nil {
		cycles = dbcatalogbuild.EOLClient{}
	}
	return &dbcatalogbuild.Builder{Tags: tags, Cycles: cycles, Accept: acceptsVersion, Now: r.cfg.Now}
}

// LiveRefresh looks one engine up in its registry and endoflife.date now.
// A lookup inside the per-engine cooldown returns the cached view with
// cooled set instead of calling upstream. A failure keeps the previous list.
func (r *RemoteCatalog) LiveRefresh(ctx context.Context, engine string) (view VersionsView, cooled bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reason := r.remoteOffReason(); reason != "" {
		return VersionsView{}, false, fmt.Errorf("%w: %s", ErrRemoteDisabled, reason)
	}
	spec, ok := dbcatalogbuild.SpecFor(engine)
	if !ok {
		return VersionsView{}, false, ErrUnsupportedEngine
	}
	now := r.cfg.Now()
	if last := r.st.LiveChecked[engine]; !last.IsZero() && now.Sub(last) < liveCooldown {
		v, verr := r.versionsLocked(engine)
		return v, true, verr
	}
	ctx, cancel := context.WithTimeout(ctx, upstreamTimeout)
	defer cancel()
	e, err := r.builder().BuildEngine(ctx, spec)
	if err != nil {
		return VersionsView{}, false, err
	}
	var prev *dbcatalogbuild.File
	if old, _, _, _, ok := r.engineData(engine); ok {
		prev = &dbcatalogbuild.File{Schema: dbcatalogbuild.SchemaVersion, Engines: map[string]dbcatalogbuild.Engine{engine: old}}
	}
	next := dbcatalogbuild.File{Schema: dbcatalogbuild.SchemaVersion, Engines: map[string]dbcatalogbuild.Engine{engine: e}}
	if err := dbcatalogbuild.ValidateAgainst(next, prev); err != nil {
		return VersionsView{}, false, fmt.Errorf("dbupgrade: live result rejected: %w", err)
	}
	if r.st.Live == nil {
		r.st.Live = map[string]*liveEntry{}
	}
	if r.st.LiveChecked == nil {
		r.st.LiveChecked = map[string]time.Time{}
	}
	r.st.Live[engine] = &liveEntry{Engine: e, FetchedAt: now}
	r.st.LiveChecked[engine] = now
	r.applyLocked()
	if err := r.saveStateLocked(); err != nil {
		r.cfg.Logger.Warn("dbupgrade: could not persist live catalog result", "error", err.Error())
	}
	v, verr := r.versionsLocked(engine)
	return v, false, verr
}

// RefreshDirect builds the whole catalog from the registries.
func (r *RemoteCatalog) RefreshDirect(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reason := r.remoteOffReason(); reason != "" {
		return fmt.Errorf("%w: %s", ErrRemoteDisabled, reason)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	f, err := r.builder().Build(ctx, dbcatalogbuild.DefaultSpecs())
	if err == nil {
		err = dbcatalogbuild.ValidateAgainst(f, r.st.Direct)
	}
	if err != nil {
		r.fail(err)
		return fmt.Errorf("dbupgrade: direct catalog build: %w", err)
	}
	now := r.cfg.Now()
	r.st.Direct, r.st.DirectAt = &f, now
	r.st.LastChecked, r.st.FailCount, r.st.LastError = now, 0, ""
	r.st.NextAttempt = now.Add(jitter(directInterval))
	r.st.Live = nil
	r.applyLocked()
	return r.saveStateLocked()
}

// Refresh runs the configured source's refresh now.
func (r *RemoteCatalog) Refresh(ctx context.Context) error {
	if r.cfg.Source == CatalogSourceDirect {
		return r.RefreshDirect(ctx)
	}
	return r.RefreshPublished(ctx)
}

func jitter(d time.Duration) time.Duration {
	return d - d/10 + time.Duration(rand.Int64N(int64(d/5)+1)) //nolint:gosec,forbidigo // jitter only, not security sensitive
}

// Run refreshes on schedule until ctx ends. It waits before the first
// attempt so startup never depends on the network.
func (r *RemoteCatalog) Run(ctx context.Context) {
	wait := startupDelay
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = r.step(ctx)
	}
}

func (r *RemoteCatalog) step(ctx context.Context) time.Duration {
	r.mu.Lock()
	off := r.publishedOffReason()
	if r.cfg.Source == CatalogSourceDirect {
		off = r.remoteOffReason()
	}
	next := r.st.NextAttempt
	now := r.cfg.Now()
	r.mu.Unlock()
	if off != "" {
		return time.Hour
	}
	if next.After(now) {
		return next.Sub(now)
	}
	if err := r.Refresh(ctx); err != nil && !errors.Is(err, ErrRemoteDisabled) {
		r.cfg.Logger.Warn("dbupgrade: catalog refresh failed, keeping the last good copy", "error", err.Error())
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if d := r.st.NextAttempt.Sub(r.cfg.Now()); d > 0 {
		return d
	}
	return time.Hour
}
