package alerting

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// History event names.
const (
	EventFired      = "fired"
	EventResolved   = "resolved"
	EventFlapping   = "flapping"
	EventFlapEnded  = "flap_ended"
	EventGroupFlush = "group"
)

// History outcome names: what happened to the notification.
const (
	OutcomeSent        = "sent"
	OutcomeSilenced    = "silenced"
	OutcomeGrouped     = "grouped"
	OutcomeInhibited   = "inhibited"
	OutcomeFailed      = "failed"
	OutcomeRateLimited = "ratelimited"
	OutcomeFlapping    = "flapping"
	OutcomeSkipped     = "skipped"
)

// HistoryEntry is one alert state change and what became of its notification.
type HistoryEntry struct {
	ID         string
	At         time.Time
	RuleID     string
	RuleName   string
	RuleKind   string
	ResourceID string
	App        string
	Node       string
	Severity   string
	Event      string
	Outcome    string
	Detail     string
	SilenceID  string
	ChannelID  string
	Error      string
}

// HistoryFilter narrows ListHistory. Zero fields match everything.
type HistoryFilter struct {
	App     string
	RuleID  string
	Outcome string
	Event   string
	Since   time.Time
	Before  *time.Time
	Limit   int
}

// NewHistoryID generates a random history identifier.
func NewHistoryID() (string, error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alerting: generate history id: %w", err)
	}
	return "ah_" + hex.EncodeToString(b), nil
}

// RecordHistory appends one entry. Insert-only.
func (db *DB) RecordHistory(ctx context.Context, e HistoryEntry) error {
	if e.ID == "" {
		id, err := NewHistoryID()
		if err != nil {
			return err
		}
		e.ID = id
	}
	if e.At.IsZero() {
		e.At = time.Now()
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO alert_history (id, at, rule_id, rule_name, rule_kind, resource_id, app, node, severity, event, outcome, detail, silence_id, channel_id, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, fmtTime(e.At), e.RuleID, e.RuleName, e.RuleKind, e.ResourceID, e.App, e.Node, e.Severity,
		e.Event, e.Outcome, e.Detail, e.SilenceID, e.ChannelID, e.Error)
	if err != nil {
		return fmt.Errorf("alerting: record history for rule %q: %w", e.RuleID, err)
	}
	return nil
}

// ListHistory returns entries newest first.
func (db *DB) ListHistory(ctx context.Context, f HistoryFilter) ([]HistoryEntry, error) {
	limit := f.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var (
		where []string
		args  []any
	)
	add := func(cond string, v any) {
		where = append(where, cond)
		args = append(args, v)
	}
	if f.App != "" {
		add("app = ?", f.App)
	}
	if f.RuleID != "" {
		add("rule_id = ?", f.RuleID)
	}
	if f.Outcome != "" {
		add("outcome = ?", f.Outcome)
	}
	if f.Event != "" {
		add("event = ?", f.Event)
	}
	if !f.Since.IsZero() {
		add("at >= ?", fmtTime(f.Since))
	}
	if f.Before != nil {
		add("at < ?", fmtTime(*f.Before))
	}
	q := `SELECT id, at, rule_id, rule_name, rule_kind, resource_id, app, node, severity, event, outcome, detail, silence_id, channel_id, error FROM alert_history`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += " ORDER BY at DESC, id DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("alerting: list history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []HistoryEntry
	for rows.Next() {
		var (
			e  HistoryEntry
			at string
		)
		if err := rows.Scan(&e.ID, &at, &e.RuleID, &e.RuleName, &e.RuleKind, &e.ResourceID, &e.App, &e.Node, &e.Severity,
			&e.Event, &e.Outcome, &e.Detail, &e.SilenceID, &e.ChannelID, &e.Error); err != nil {
			return nil, fmt.Errorf("alerting: scan history row: %w", err)
		}
		if e.At, err = time.Parse(time.RFC3339Nano, at); err != nil {
			return nil, fmt.Errorf("alerting: parse history time: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("alerting: iterate history rows: %w", err)
	}
	return out, nil
}

// PruneHistory deletes entries older than cutoff and returns how many went.
func (db *DB) PruneHistory(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM alert_history WHERE at < ?`, fmtTime(cutoff))
	if err != nil {
		return 0, fmt.Errorf("alerting: prune history: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("alerting: prune history rows: %w", err)
	}
	return n, nil
}
