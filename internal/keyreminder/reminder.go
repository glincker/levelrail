// Package keyreminder sends a scheduled notice when encryption keys are
// older than the operator's maximum key age policy.
package keyreminder

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/ticker"
)

// DefaultInterval is how often the policy is checked, not how often a notice goes out.
const DefaultInterval = 6 * time.Hour

const maxNamedApps = 5

// Source is the store surface the reminder reads. *store.DB satisfies it.
type Source interface {
	GetSecretKeyPolicy(ctx context.Context) (store.SecretKeyPolicy, error)
	MarkSecretKeyReminded(ctx context.Context, at time.Time) error
	ListServiceDEKStatus(ctx context.Context) ([]store.ServiceDEKStatus, error)
	GetMasterKeyRotatedAt(ctx context.Context) (time.Time, bool, error)
}

// Channels lists notification channels. *alerting.DB satisfies it.
type Channels interface {
	ListNotificationChannels(ctx context.Context) ([]alerting.NotificationChannel, error)
}

// Notifier sends one plain-text notice. *alerting.DeployDispatcher satisfies it.
type Notifier interface {
	SendNotice(ctx context.Context, c alerting.NotificationChannel, text string) error
}

// Reminder notifies about keys past the policy's maximum age.
type Reminder struct {
	Source   Source
	Channels Channels
	Notifier Notifier
	Logger   *slog.Logger
	Now      func() time.Time
}

func (r *Reminder) now() time.Time {
	if r.Now != nil {
		return r.Now().UTC()
	}
	return time.Now().UTC()
}

// Run ticks until ctx is done.
func (r *Reminder) Run(ctx context.Context, interval time.Duration) error {
	if interval <= 0 {
		interval = DefaultInterval
	}
	return ticker.Run(ctx, interval, r.Logger, "keyreminder", r.Tick)
}

// Tick sends one notice when something is overdue and the last notice is
// older than the policy's repeat interval. It stays unmarked when no
// channel accepted it, so the next tick retries.
func (r *Reminder) Tick(ctx context.Context) error {
	policy, err := r.Source.GetSecretKeyPolicy(ctx)
	if err != nil {
		return fmt.Errorf("keyreminder: load policy: %w", err)
	}
	if policy.MaxKeyAgeDays <= 0 {
		return nil
	}
	now := r.now()
	repeat := time.Duration(policy.RemindEveryDays) * 24 * time.Hour
	if !policy.LastRemindedAt.IsZero() && now.Sub(policy.LastRemindedAt) < repeat {
		return nil
	}
	limit := time.Duration(policy.MaxKeyAgeDays) * 24 * time.Hour

	statuses, err := r.Source.ListServiceDEKStatus(ctx)
	if err != nil {
		return fmt.Errorf("keyreminder: list keys: %w", err)
	}
	var apps []string
	for _, s := range statuses {
		if now.Sub(s.KeyAgeAnchor()) > limit {
			apps = append(apps, s.ServiceName)
		}
	}
	sort.Strings(apps)
	masterOverdue := false
	if at, ok, err := r.Source.GetMasterKeyRotatedAt(ctx); err == nil && ok {
		masterOverdue = now.Sub(at) > limit
	}
	if len(apps) == 0 && !masterOverdue {
		return nil
	}

	channels, err := r.Channels.ListNotificationChannels(ctx)
	if err != nil {
		return fmt.Errorf("keyreminder: list channels: %w", err)
	}
	text := NoticeText(policy.MaxKeyAgeDays, masterOverdue, apps)
	sent := 0
	for _, c := range channels {
		if !c.Enabled {
			continue
		}
		if err := r.Notifier.SendNotice(ctx, c, text); err != nil {
			r.Logger.Warn("keyreminder: notice failed",
				slog.String("channel_id", c.ID), slog.String("error", err.Error()))
			continue
		}
		sent++
	}
	if sent == 0 {
		return nil
	}
	return r.Source.MarkSecretKeyReminded(ctx, now)
}

// NoticeText is the plain-text reminder: names and counts, never key material.
func NoticeText(maxDays int, masterOverdue bool, apps []string) string {
	var parts []string
	if masterOverdue {
		parts = append(parts, "the master key")
	}
	if n := len(apps); n > 0 {
		shown := apps
		if n > maxNamedApps {
			shown = apps[:maxNamedApps]
		}
		label := fmt.Sprintf("%d app data key(s) (%s", n, strings.Join(shown, ", "))
		if n > maxNamedApps {
			label += fmt.Sprintf(" and %d more", n-maxNamedApps)
		}
		parts = append(parts, label+")")
	}
	return fmt.Sprintf("Encryption key reminder: %s older than the %d day limit. Rotate them under Settings, General, Encryption keys.",
		strings.Join(parts, " and "), maxDays)
}
