package api

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
)

const (
	deviceNoticeText     = "A CLI login is waiting for approval. Open Settings > CLI access in the dashboard to review it."
	deviceNoticeTimeout  = 15 * time.Second
	deviceNoticeInterval = time.Minute
)

// DeviceLoginNotifier sends one plain-text notice through a channel.
// *alerting.DeployDispatcher satisfies it.
type DeviceLoginNotifier interface {
	SendNotice(ctx context.Context, kind alerting.NotifyKind, notifyURL, text string) error
}

// deviceNoticeGate allows at most one outbound notice per interval across
// all callers, so unauthenticated starts from many IPs cannot spam a channel.
type deviceNoticeGate struct {
	mu   sync.Mutex
	last time.Time
}

func (g *deviceNoticeGate) take(now time.Time) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.last.IsZero() && now.Sub(g.last) < deviceNoticeInterval {
		return false
	}
	g.last = now
	return true
}

// notifyDeviceLoginWaiting tells opted-in channels a CLI login awaits
// approval. The text never carries the code, and the link is only added
// from the configured dashboard URL, never from request headers.
func (rt *Router) notifyDeviceLoginWaiting() {
	if rt.deviceNotifier == nil || rt.notificationChannels == nil || !rt.deviceNotices.take(time.Now()) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), deviceNoticeTimeout)
		defer cancel()
		channels, err := rt.notificationChannels.ListNotificationChannels(ctx)
		if err != nil {
			rt.logger.Error("api: list notification channels for device login notice failed", slog.String("error", err.Error()))
			return
		}
		text := deviceNoticeText
		if base := rt.dashboardURL(); base != "" {
			text += " " + base + deviceCLIPath
		}
		for _, c := range channels {
			if !c.Enabled || !c.NotifyDeviceLogin {
				continue
			}
			if err := rt.deviceNotifier.SendNotice(ctx, c.Kind, c.NotifyURL, text); err != nil {
				rt.logger.Error("api: send device login notice failed",
					slog.String("channel_id", c.ID), slog.String("error", err.Error()))
			}
		}
	}()
}

func (rt *Router) dashboardURL() string {
	if base := configuredDashboardURL(); base != "" {
		return base
	}
	return rt.publicDashboardURL
}
