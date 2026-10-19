package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/footprint"
)

const digestWindow = 24 * time.Hour

// DigestSender is the slice of DeployDispatcher the digest needs.
type DigestSender interface {
	SendNotice(ctx context.Context, c NotificationChannel, text string) error
}

// DigestRunner sends each opted-in principal one summary of the last day's inbox.
type DigestRunner struct {
	DB     *DB
	Sender DigestSender
	Logger *slog.Logger
	Now    func() time.Time
}

// FormatDigest renders items as a short plain-text summary, or "" when there is nothing to say.
func FormatDigest(items []InboxItem) string {
	if len(items) == 0 {
		return ""
	}
	var crit, warn, info, unread int
	for _, it := range items {
		switch it.Severity {
		case SeverityCritical:
			crit++
		case SeverityInfo:
			info++
		default:
			warn++
		}
		if !it.Read {
			unread++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Daily alert digest: %d events (%d critical, %d warning, %d info), %d unread.", len(items), crit, warn, info, unread)
	shown := items
	if len(shown) > 10 {
		shown = shown[:10]
	}
	for _, it := range shown {
		target := it.App
		if target == "" {
			target = it.Node
		}
		fmt.Fprintf(&b, "\n- %s %s %s %s", it.At.UTC().Format("15:04"), it.Severity, it.Event, strings.TrimSpace(it.RuleName+" "+target))
	}
	if len(items) > len(shown) {
		fmt.Fprintf(&b, "\n...and %d more in the inbox.", len(items)-len(shown))
	}
	return b.String()
}

// RunOnce sends every due digest and returns how many went out. A principal with an empty day is marked sent without a message.
func (r *DigestRunner) RunOnce(ctx context.Context) (int, error) {
	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}
	due, err := r.DB.DigestDue(ctx, now)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, p := range due {
		items, err := r.DB.ListInbox(ctx, p.Principal, InboxFilter{Limit: inboxMaxLimit})
		if err != nil {
			r.log().Error("alerting: digest list failed", slog.String("principal", p.Principal), slog.String("error", err.Error()))
			continue
		}
		recent := items[:0]
		for _, it := range items {
			if now.Sub(it.At) <= digestWindow {
				recent = append(recent, it)
			}
		}
		if text := FormatDigest(recent); text != "" {
			ch, err := r.DB.GetNotificationChannel(ctx, p.DigestChannelID)
			if err != nil {
				r.log().Error("alerting: digest channel missing", slog.String("principal", p.Principal), slog.String("error", err.Error()))
				continue
			}
			if err := r.Sender.SendNotice(ctx, *ch, text); err != nil {
				r.log().Error("alerting: digest send failed", slog.String("principal", p.Principal), slog.String("error", err.Error()))
				continue
			}
			sent++
		}
		if err := r.DB.MarkDigestSent(ctx, p.Principal, now); err != nil {
			return sent, err
		}
	}
	return sent, nil
}

// Run checks for due digests every interval until ctx is done.
func (r *DigestRunner) Run(ctx context.Context, interval time.Duration) error {
	loop := footprint.Track("inbox digest", interval)
	defer loop.Active()()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if _, err := r.RunOnce(ctx); err != nil {
				r.log().Error("alerting: digest run failed", slog.String("error", err.Error()))
			}
		}
	}
}

func (r *DigestRunner) log() *slog.Logger {
	if r.Logger != nil {
		return r.Logger
	}
	return slog.Default()
}
