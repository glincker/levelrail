// Package updatecheck periodically checks the configured update channel's
// latest release against the running build, when an operator has opted
// into auto-update checking (update_settings.auto_update_enabled,
// migrations/0258_update_settings.sql). It only checks and records the
// result; it never applies an update itself.
package updatecheck

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/upgrade"
	"github.com/GLINCKER/levelrail/internal/version"
)

// Store is the narrow store surface Scheduler needs. *store.DB satisfies
// this structurally, the same "consumer defines its own narrow
// interface" convention internal/backup.ScheduleStore already
// establishes for its own store dependency.
type Store interface {
	GetUpdateSettings(ctx context.Context) (store.UpdateSettings, error)
}

// Result is the outcome of the most recently completed check.
type Result struct {
	Channel         string
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
	ReleaseURL      string
	CheckedAt       time.Time
}

// Scheduler checks, on its own tick, whether update_settings has
// auto-update enabled and, if so, fetches the configured channel's
// latest release via Fetchers (internal/upgrade), the identical
// lookup+comparison logic GET /api/v1/updates itself uses
// (internal/api/updates.go's handleGetUpdates). Run's own tick-forever
// shape mirrors backup.Scheduler.Run/alerting.Engine.Run.
type Scheduler struct {
	Store    Store
	Fetchers upgrade.Fetchers
	Logger   *slog.Logger

	mu     sync.Mutex
	result Result
}

// NewScheduler builds a Scheduler ready to Tick or Run. logger defaults
// to slog.Default() if nil, the same convention backup.NewScheduler
// already establishes.
func NewScheduler(st Store, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Scheduler{Store: st, Fetchers: upgrade.DefaultFetchers(), Logger: logger}
}

func (s *Scheduler) log() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}

// Result returns the outcome of the last completed check, the zero
// Result if none has run yet (or auto-update has never been enabled).
func (s *Scheduler) Result() Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}

// Tick reads update_settings and, only when auto_update_enabled is set,
// fetches the configured channel's latest release and records it.
// A disabled setting is not an error: Tick is simply a no-op until an
// operator opts in, the same "absence is a real, valid state" stance
// this codebase takes elsewhere (e.g. IngressSettings.PrimaryDomain).
func (s *Scheduler) Tick(ctx context.Context) error {
	settings, err := s.Store.GetUpdateSettings(ctx)
	if err != nil {
		return fmt.Errorf("updatecheck: get update settings: %w", err)
	}
	if !settings.AutoUpdateEnabled {
		return nil
	}

	channel := settings.Channel
	if !upgrade.ValidChannel(channel) {
		channel = upgrade.ChannelStable
	}

	release, err := s.Fetchers.LatestForChannel(ctx, channel)
	if err != nil {
		return fmt.Errorf("updatecheck: fetch latest release for channel %q: %w", channel, err)
	}

	result := Result{Channel: channel, CurrentVersion: version.Version, CheckedAt: time.Now()}
	if release != nil {
		result.LatestVersion = release.Tag
		result.ReleaseURL = release.URL
		result.UpdateAvailable = upgrade.UpdateAvailable(version.Version, release)
	}

	s.mu.Lock()
	s.result = result
	s.mu.Unlock()

	if result.UpdateAvailable {
		s.log().Info("updatecheck: update available",
			slog.String("channel", channel), slog.String("current_version", version.Version), slog.String("latest_version", result.LatestVersion))
	}
	return nil
}

// Run calls Tick on interval until ctx is done, matching the shape of
// every other periodic loop in this codebase (backup.Scheduler.Run,
// alerting.Engine.Run, telemetry.Collector.Run).
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				s.log().Warn("updatecheck: scheduler tick failed", slog.String("error", err.Error()))
			}
		}
	}
}
