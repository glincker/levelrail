package maintenance

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/footprint"
	"github.com/GLINCKER/levelrail/internal/store"
)

// DefaultInterval is how often the announcer looks for windows entering
// their lead time.
const DefaultInterval = time.Minute

// AnnounceStore is the store surface the announcer needs.
type AnnounceStore interface {
	ListMaintenanceWindows(ctx context.Context, since time.Time) ([]store.MaintenanceWindow, error)
	MarkMaintenanceAnnounced(ctx context.Context, id string, at time.Time) error
}

// Channels lists notification channels. *alerting.DB satisfies it.
type Channels interface {
	ListNotificationChannels(ctx context.Context) ([]alerting.NotificationChannel, error)
}

// Notifier sends one plain-text notice. *alerting.DeployDispatcher satisfies it.
type Notifier interface {
	SendNotice(ctx context.Context, c alerting.NotificationChannel, text string) error
}

// Announcer sends each window's lead-time notice once.
type Announcer struct {
	Store    AnnounceStore
	Channels Channels
	Notifier Notifier
	Logger   *slog.Logger
	Now      func() time.Time
}

func (a *Announcer) now() time.Time {
	if a.Now != nil {
		return a.Now().UTC()
	}
	return time.Now().UTC()
}

// Run ticks until ctx is done.
func (a *Announcer) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return footprint.Every(ctx, "maintenance", interval, a.Logger, a.Tick)
}

// Tick announces every window whose lead time has begun. A window stays
// unmarked when no channel accepted the notice, so the next tick retries.
func (a *Announcer) Tick(ctx context.Context) error {
	now := a.now()
	windows, err := a.Store.ListMaintenanceWindows(ctx, now)
	if err != nil {
		return fmt.Errorf("maintenance: list windows: %w", err)
	}
	var due []store.MaintenanceWindow
	for _, w := range windows {
		if AnnounceDue(w, now) {
			due = append(due, w)
		}
	}
	if len(due) == 0 {
		return nil
	}
	channels, err := a.Channels.ListNotificationChannels(ctx)
	if err != nil {
		return fmt.Errorf("maintenance: list channels: %w", err)
	}
	for _, w := range due {
		sent, enabled := 0, 0
		for _, c := range channels {
			if !c.Enabled {
				continue
			}
			enabled++
			if err := a.Notifier.SendNotice(ctx, c, NoticeText(w)); err != nil {
				a.Logger.Warn("maintenance: announce failed",
					slog.String("window_id", w.ID), slog.String("channel_id", c.ID), slog.String("error", err.Error()))
				continue
			}
			sent++
		}
		if enabled > 0 && sent == 0 {
			continue
		}
		if err := a.Store.MarkMaintenanceAnnounced(ctx, w.ID, now); err != nil {
			return err
		}
	}
	return nil
}

// NoticeText is the plain-text announcement for w.
func NoticeText(w store.MaintenanceWindow) string {
	const layout = "2006-01-02 15:04 MST"
	text := fmt.Sprintf("Scheduled maintenance: %s, %s to %s.", w.Title,
		w.StartsAt.UTC().Format(layout), w.EndsAt.UTC().Format(layout))
	if w.Freeze {
		text += " Automatic deploys are held during the window."
	}
	if w.Message != "" {
		text += " " + w.Message
	}
	return text
}
