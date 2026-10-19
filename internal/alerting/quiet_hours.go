package alerting

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// QuietHours holds back non-critical firing notifications on one channel
// between Start and End (HH:MM, evaluated in Timezone). A window that ends
// before it starts spans midnight.
type QuietHours struct {
	ChannelID string
	Start     string
	End       string
	Timezone  string
	Enabled   bool
}

const quietClockLayout = "15:04"

func parseQuietClock(s string) (int, error) {
	t, err := time.Parse(quietClockLayout, s)
	if err != nil {
		return 0, fmt.Errorf("%q is not a time of day as HH:MM", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

// Validate rejects unparsable times, an empty window and unknown zones.
func (q QuietHours) Validate() error {
	start, err := parseQuietClock(q.Start)
	if err != nil {
		return fmt.Errorf("start: %w", err)
	}
	end, err := parseQuietClock(q.End)
	if err != nil {
		return fmt.Errorf("end: %w", err)
	}
	if start == end {
		return errors.New("start and end must differ")
	}
	if _, err := time.LoadLocation(q.zone()); err != nil {
		return fmt.Errorf("timezone %q is not a known zone", q.Timezone)
	}
	return nil
}

func (q QuietHours) zone() string {
	if q.Timezone == "" {
		return "UTC"
	}
	return q.Timezone
}

// ActiveAt reports whether t falls inside the quiet window.
func (q QuietHours) ActiveAt(t time.Time) bool {
	if !q.Enabled {
		return false
	}
	loc, err := time.LoadLocation(q.zone())
	if err != nil {
		return false
	}
	start, err1 := parseQuietClock(q.Start)
	end, err2 := parseQuietClock(q.End)
	if err1 != nil || err2 != nil || start == end {
		return false
	}
	local := t.In(loc)
	now := local.Hour()*60 + local.Minute()
	if start < end {
		return now >= start && now < end
	}
	return now >= start || now < end
}

// GetChannelQuietHours returns the channel's quiet hours, or nil when none are set.
func (db *DB) GetChannelQuietHours(ctx context.Context, channelID string) (*QuietHours, error) {
	q := QuietHours{ChannelID: channelID}
	var enabled int
	err := db.QueryRowContext(ctx, `SELECT start_time, end_time, timezone, enabled FROM channel_quiet_hours WHERE channel_id = ?`, channelID).
		Scan(&q.Start, &q.End, &q.Timezone, &enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("alerting: get quiet hours for %q: %w", channelID, err)
	}
	q.Enabled = enabled != 0
	return &q, nil
}

// SetChannelQuietHours creates or replaces the channel's quiet hours.
func (db *DB) SetChannelQuietHours(ctx context.Context, q QuietHours) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO channel_quiet_hours (channel_id, start_time, end_time, timezone, enabled, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (channel_id) DO UPDATE SET
			start_time = excluded.start_time, end_time = excluded.end_time,
			timezone = excluded.timezone, enabled = excluded.enabled, updated_at = excluded.updated_at`,
		q.ChannelID, q.Start, q.End, q.zone(), boolToInt(q.Enabled), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("alerting: set quiet hours for %q: %w", q.ChannelID, err)
	}
	return nil
}

// DeleteChannelQuietHours removes the channel's quiet hours; absent is not an error.
func (db *DB) DeleteChannelQuietHours(ctx context.Context, channelID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM channel_quiet_hours WHERE channel_id = ?`, channelID); err != nil {
		return fmt.Errorf("alerting: delete quiet hours for %q: %w", channelID, err)
	}
	return nil
}

type quietHoursSource interface {
	GetChannelQuietHours(ctx context.Context, channelID string) (*QuietHours, error)
}

// quietNow reports whether r's channel is in quiet hours and r is not critical.
func (n *NoiseControl) quietNow(ctx context.Context, r Rule, now time.Time) bool {
	src, ok := n.store.(quietHoursSource)
	if !ok || r.ChannelID == "" || severityOrDefault(r.Severity) == SeverityCritical {
		return false
	}
	q, err := src.GetChannelQuietHours(ctx, r.ChannelID)
	if err != nil {
		n.logger.Warn("alerting: load quiet hours failed, sending normally", slog.String("channel_id", r.ChannelID), slog.String("error", err.Error()))
		return false
	}
	return q != nil && q.ActiveAt(now)
}
