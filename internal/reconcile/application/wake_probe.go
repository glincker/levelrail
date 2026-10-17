package application

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/probe"
)

// WakeWindowReader tells whether an app was woken from sleep recently.
type WakeWindowReader interface {
	GetAppSleep(ctx context.Context, serviceName string) (store.AppSleep, error)
}

// WithWakeProbe probes a freshly woken app's readiness every interval instead
// of the app's own (slower) interval, so held requests are released on the
// first ready signal. Later checks use the normal interval again.
func WithWakeProbe(r WakeWindowReader, interval time.Duration) Option {
	return func(c *Controller) { c.wakeWindow, c.wakeProbeInterval = r, interval }
}

// wakeProbeConfig shortens cfg.Interval while desired is inside its wake window.
func (c *Controller) wakeProbeConfig(ctx context.Context, desired *store.DesiredService, cfg probe.Config) probe.Config {
	if c.wakeWindow == nil || c.wakeProbeInterval <= 0 {
		return cfg
	}
	a, err := c.wakeWindow.GetAppSleep(ctx, desired.Name)
	if err != nil || a.IdleMinutes <= 0 || a.Sleeping || a.Since.IsZero() {
		return cfg
	}
	if time.Since(a.Since) > c.effectiveReadyBudget(desired) {
		return cfg
	}
	if cfg.Interval <= 0 || cfg.Interval > c.wakeProbeInterval {
		cfg.Interval = c.wakeProbeInterval
	}
	return cfg
}
