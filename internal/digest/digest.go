// Package digest builds a per-project weekly summary of deploys, failures,
// fired alerts and backup status and sends it through notification channels.
package digest

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

// Stats is what one project did inside the digest window.
type Stats struct {
	Project       string
	Deploys       int
	Failures      int
	AlertsFired   int
	BackupsOK     int
	BackupsFailed int
}

// Quiet reports whether there is nothing worth sending for the project.
func (s Stats) Quiet() bool {
	return s.Deploys == 0 && s.Failures == 0 && s.AlertsFired == 0 && s.BackupsOK == 0 && s.BackupsFailed == 0
}

// Source supplies the window stats and the channels a project digest goes to.
type Source interface {
	ProjectStats(ctx context.Context, since, until time.Time) ([]Stats, error)
	ChannelsFor(ctx context.Context, project string) ([]string, error)
}

// Sender delivers one text notice to a channel id.
type Sender interface {
	SendNotice(ctx context.Context, channelID, text string) error
}

// Format renders one project's digest as short plain text.
func Format(s Stats, since, until time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Weekly digest for %s (%s to %s)", s.Project, since.UTC().Format("2006-01-02"), until.UTC().Format("2006-01-02"))
	fmt.Fprintf(&b, "\n- Deploys: %d (%d failed)", s.Deploys, s.Failures)
	fmt.Fprintf(&b, "\n- Alerts fired: %d", s.AlertsFired)
	fmt.Fprintf(&b, "\n- Backups: %d ok, %d failed", s.BackupsOK, s.BackupsFailed)
	return b.String()
}

// Runner sends one digest per active project. Interval and enablement are
// read per tick through the getters so runtime settings apply live.
type Runner struct {
	Source   Source
	Sender   Sender
	Logger   *slog.Logger
	Now      func() time.Time
	Enabled  func() bool
	Interval func() time.Duration
}

// RunOnce sends digests for the window ending now. It returns how many
// notices went out; a failing channel is logged and does not stop the rest.
func (r *Runner) RunOnce(ctx context.Context) (int, error) {
	if r.Enabled != nil && !r.Enabled() {
		return 0, nil
	}
	now := r.Now()
	window := 7 * 24 * time.Hour
	if r.Interval != nil {
		if d := r.Interval(); d > 0 {
			window = d
		}
	}
	since := now.Add(-window)
	all, err := r.Source.ProjectStats(ctx, since, now)
	if err != nil {
		return 0, fmt.Errorf("digest stats: %w", err)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Project < all[j].Project })
	sent := 0
	for _, s := range all {
		if s.Quiet() {
			continue
		}
		chans, err := r.Source.ChannelsFor(ctx, s.Project)
		if err != nil {
			r.Logger.Warn("digest channels", "project", s.Project, "error", err)
			continue
		}
		text := Format(s, since, now)
		for _, id := range chans {
			if err := r.Sender.SendNotice(ctx, id, text); err != nil {
				r.Logger.Warn("digest send", "project", s.Project, "channel", id, "error", err)
				continue
			}
			sent++
		}
	}
	return sent, nil
}

// Run ticks until ctx ends, calling RunOnce each interval.
func (r *Runner) Run(ctx context.Context) {
	for {
		wait := 7 * 24 * time.Hour
		if r.Interval != nil {
			if d := r.Interval(); d > 0 {
				wait = d
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
			if _, err := r.RunOnce(ctx); err != nil {
				r.Logger.Warn("digest run", "error", err)
			}
		}
	}
}
