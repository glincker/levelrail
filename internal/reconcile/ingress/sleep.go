package ingress

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
)

// SleepSource lists apps with a sleep-when-idle setting.
type SleepSource interface {
	ListAppSleep(ctx context.Context) ([]store.AppSleep, error)
}

// WithAppWake serves a waking-up page for sleeping apps. dial is the control
// plane listener the page's wake request is proxied to, token its shared secret.
func WithAppWake(src SleepSource, dial, token string) Option {
	return func(c *Controller) { c.sleepSource, c.wakeDial, c.wakeToken = src, dial, token }
}

// sleepingApps returns the names of apps asleep right now.
func (c *Controller) sleepingApps(ctx context.Context) map[string]bool {
	if c.sleepSource == nil || c.wakeDial == "" {
		return nil
	}
	rows, err := c.sleepSource.ListAppSleep(ctx)
	if err != nil {
		c.logger.WarnContext(ctx, "ingress: list sleeping apps failed, treating none as asleep", slog.String("error", err.Error()))
		return nil
	}
	out := make(map[string]bool, len(rows))
	for _, r := range rows {
		if r.Sleeping {
			out[r.ServiceName] = true
		}
	}
	return out
}

func (c *Controller) wakeRoute(name string, hosts []string) ingress.WakeRoute {
	return ingress.WakeRoute{Hosts: hosts, Dial: c.wakeDial, Token: c.wakeToken, App: name}
}
