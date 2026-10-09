// Package imageupdate periodically asks the control plane to check each
// opted-in app's image tag against its registry and redeploy when it moved.
package imageupdate

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EnvInterval overrides DefaultInterval (a Go duration such as "30m").
const EnvInterval = "APP_IMAGE_UPDATE_INTERVAL"

// DefaultInterval is how often opted-in apps are checked. Hourly: registries
// rate-limit anonymous digest lookups, and a tag rarely moves faster.
const DefaultInterval = time.Hour

// Store lists the apps that opted in.
type Store interface {
	ListEnabledImageAutoUpdates(ctx context.Context) ([]store.ImageAutoUpdate, error)
}

// Checker runs one app's update check; *api.Router satisfies it.
type Checker interface {
	CheckImageUpdate(ctx context.Context, serviceName string) (string, error)
}

// Scheduler checks every opted-in app once per Tick.
type Scheduler struct {
	Store   Store
	Checker Checker
	Logger  *slog.Logger
}

// IntervalFromEnv returns EnvInterval as a duration, else DefaultInterval.
func IntervalFromEnv(logger *slog.Logger) time.Duration {
	raw := os.Getenv(EnvInterval)
	if raw == "" {
		return DefaultInterval
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		logger.Warn("invalid "+EnvInterval+", using the default", slog.String("value", raw))
		return DefaultInterval
	}
	return d
}

// Tick checks each opted-in app; one app's failure never stops the rest.
func (s *Scheduler) Tick(ctx context.Context) error {
	apps, err := s.Store.ListEnabledImageAutoUpdates(ctx)
	if err != nil {
		return fmt.Errorf("imageupdate: list opted-in apps: %w", err)
	}
	var errs []error
	for _, a := range apps {
		result, err := s.Checker.CheckImageUpdate(ctx, a.ServiceName)
		if err != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", a.ServiceName, err))
			continue
		}
		s.Logger.Info("imageupdate: checked", slog.String("app", a.ServiceName), slog.String("result", result))
	}
	return errors.Join(errs...)
}

// Run ticks on interval until ctx is done.
func (s *Scheduler) Run(ctx context.Context, interval time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				s.Logger.Warn("imageupdate: tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}
