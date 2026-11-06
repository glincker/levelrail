package diskhistory

import (
	"context"
	"log/slog"
	"time"
)

// Categories are the per-category byte totals taken from the Docker usage cache.
type Categories struct {
	Images, Containers, Volumes, BuildCache int64
}

// Source supplies live readings. Usage may be nil or fail; Stat is required.
type Source struct {
	Stat  func() (free, total int64, err error)
	Usage func(ctx context.Context) (Categories, error)
}

// Settings reads the live tunables on every tick so a saved change applies without a restart.
type Settings struct {
	Interval  func() time.Duration
	Retention func() time.Duration
}

// Sampler records one sample per interval, plus one at boot when the last is stale.
type Sampler struct {
	store    *Store
	src      Source
	settings Settings
	logger   *slog.Logger
	now      func() time.Time
	poll     time.Duration
}

// NewSampler builds a sampler. poll is how often it re-checks the interval and prunes.
func NewSampler(store *Store, src Source, settings Settings, poll time.Duration, logger *slog.Logger) *Sampler {
	return &Sampler{store: store, src: src, settings: settings, logger: logger, now: time.Now, poll: poll}
}

// Tick records a sample when none exists within the interval, then prunes past retention.
func (s *Sampler) Tick(ctx context.Context) error {
	last, err := s.store.Latest(ctx)
	if err != nil {
		return err
	}
	now := s.now()
	if last.IsZero() || now.Sub(last) >= s.settings.Interval() {
		if err := s.record(ctx, now); err != nil {
			return err
		}
	}
	_, err = s.store.Prune(ctx, now.Add(-s.settings.Retention()))
	return err
}

func (s *Sampler) record(ctx context.Context, now time.Time) error {
	free, total, err := s.src.Stat()
	if err != nil || total <= 0 {
		s.logger.Warn("disk history sample skipped", slog.Any("error", err))
		return nil
	}
	sm := Sample{At: now, FreeBytes: free, TotalBytes: total}
	if s.src.Usage != nil {
		if c, uerr := s.src.Usage(ctx); uerr == nil {
			sm.ImagesBytes, sm.ContainersBytes, sm.VolumesBytes, sm.BuildCacheBytes = c.Images, c.Containers, c.Volumes, c.BuildCache
		}
	}
	return s.store.Insert(ctx, sm)
}

// Run ticks at boot and then every poll until ctx ends.
func (s *Sampler) Run(ctx context.Context) {
	t := time.NewTicker(s.poll)
	defer t.Stop()
	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("disk history tick failed", slog.String("error", err.Error()))
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
