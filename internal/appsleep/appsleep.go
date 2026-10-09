// Package appsleep stops opted-in apps that have had no requests for a while
// and wakes them on the next request, so idle apps cost no memory or CPU.
package appsleep

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// DefaultInterval is how often idle apps are looked for.
const DefaultInterval = time.Minute

// SleepStore is the sleep settings table.
type SleepStore interface {
	ListAppSleep(ctx context.Context) ([]store.AppSleep, error)
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
}

// Apps is the slice of the app store the scheduler needs.
type Apps interface {
	GetDesiredService(ctx context.Context, name string) (*store.DesiredService, error)
	UpdateServiceSuspended(ctx context.Context, name string, suspended bool) error
}

// Traffic reports each app's most recent request sample.
type Traffic interface {
	LatestByMetric(ctx context.Context, metric string) ([]telemetry.Sample, error)
}

// Nudger asks the reconcilers to run now.
type Nudger interface{ Nudge() }

// Scheduler puts idle opted-in apps to sleep.
type Scheduler struct {
	Sleep   SleepStore
	Apps    Apps
	Traffic Traffic
	Nudger  Nudger
	Logger  *slog.Logger
	Now     func() time.Time
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Tick sleeps every opted-in app idle past its threshold and clears the
// sleeping flag of any app someone started by hand.
func (s *Scheduler) Tick(ctx context.Context) error {
	settings, err := s.Sleep.ListAppSleep(ctx)
	if err != nil {
		return fmt.Errorf("appsleep: list settings: %w", err)
	}
	samples, err := s.Traffic.LatestByMetric(ctx, telemetry.MetricHTTPRequests)
	if err != nil {
		return fmt.Errorf("appsleep: read request metrics: %w", err)
	}
	lastRequest := make(map[string]time.Time, len(samples))
	for _, smp := range samples {
		lastRequest[smp.ResourceID] = smp.Timestamp
	}
	now := s.now()
	var errs []error
	for _, a := range settings {
		svc, err := s.Apps.GetDesiredService(ctx, a.ServiceName)
		if errors.Is(err, store.ErrServiceNotFound) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", a.ServiceName, err))
			continue
		}
		if a.Sleeping {
			if !svc.Suspended {
				errs = append(errs, s.Sleep.SetAppSleeping(ctx, a.ServiceName, false, now))
			}
			continue
		}
		if svc.Suspended {
			continue
		}
		idleSince := a.Since
		if t := lastRequest["service:"+a.ServiceName]; t.After(idleSince) {
			idleSince = t
		}
		if now.Sub(idleSince) < time.Duration(a.IdleMinutes)*time.Minute {
			continue
		}
		if err := s.sleep(ctx, a.ServiceName, now); err != nil {
			errs = append(errs, fmt.Errorf("app %q: %w", a.ServiceName, err))
		}
	}
	return errors.Join(errs...)
}

func (s *Scheduler) sleep(ctx context.Context, name string, now time.Time) error {
	if err := s.Sleep.SetAppSleeping(ctx, name, true, now); err != nil {
		return err
	}
	if err := s.Apps.UpdateServiceSuspended(ctx, name, true); err != nil {
		_ = s.Sleep.SetAppSleeping(ctx, name, false, now)
		return fmt.Errorf("stop idle app: %w", err)
	}
	if s.Nudger != nil {
		s.Nudger.Nudge()
	}
	s.Logger.Info("appsleep: app idle, stopped", slog.String("app", name))
	return nil
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
				s.Logger.Warn("appsleep: tick had errors", slog.String("error", err.Error()))
			}
		}
	}
}

// Wake starts a sleeping app and resets its idle clock. It reports whether the
// app was actually asleep, so repeated wake requests are harmless.
func Wake(ctx context.Context, sleep interface {
	SetAppSleeping(ctx context.Context, serviceName string, sleeping bool, now time.Time) error
	GetAppSleep(ctx context.Context, serviceName string) (store.AppSleep, error)
}, apps Apps, nudger Nudger, name string, now time.Time) (bool, error) {
	a, err := sleep.GetAppSleep(ctx, name)
	if err != nil {
		return false, err
	}
	if !a.Sleeping {
		return false, nil
	}
	if err := apps.UpdateServiceSuspended(ctx, name, false); err != nil {
		return false, fmt.Errorf("start sleeping app: %w", err)
	}
	if err := sleep.SetAppSleeping(ctx, name, false, now); err != nil {
		return false, err
	}
	if nudger != nil {
		nudger.Nudge()
	}
	return true, nil
}
