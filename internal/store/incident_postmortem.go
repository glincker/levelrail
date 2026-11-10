package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Postmortem states; published is terminal.
const (
	PostmortemStateDraft     = "draft"
	PostmortemStatePublished = "published"
)

// Postmortem errors the API maps to 404 and 409.
var (
	// ErrIncidentPostmortemNotFound means the incident has no postmortem yet.
	ErrIncidentPostmortemNotFound = errors.New("store: incident postmortem not found")
	// ErrIncidentPostmortemPublished means the row is locked.
	ErrIncidentPostmortemPublished = errors.New("store: incident postmortem is published")
	// ErrIncidentPostmortemIncomplete means publish found no summary or root cause.
	ErrIncidentPostmortemIncomplete = errors.New("store: incident postmortem needs a summary and a root cause")
)

// PostmortemAlert is one alert source that fired during the impact window.
type PostmortemAlert struct {
	Kind   string     `json:"kind"`
	Ref    string     `json:"ref"`
	DownAt *time.Time `json:"down_at,omitempty"`
	UpAt   *time.Time `json:"up_at,omitempty"`
}

// IncidentPostmortem is the structured review of one incident.
type IncidentPostmortem struct {
	IncidentID  string
	Summary     string
	Impact      string
	RootCause   string
	Resolution  string
	ActionItems []string
	ImpactStart time.Time
	ImpactEnd   *time.Time
	Alerts      []PostmortemAlert
	State       string
	CreatedBy   string
	UpdatedBy   string
	PublishedBy string
	PublishedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// GetIncidentPostmortem returns the postmortem of one incident.
func (db *DB) GetIncidentPostmortem(ctx context.Context, incidentID string) (IncidentPostmortem, error) {
	var (
		p                   IncidentPostmortem
		items, alerts       string
		start, created, upd string
		end, pubAt          sql.NullString
	)
	err := db.QueryRowContext(ctx, `SELECT incident_id, summary, impact, root_cause, resolution, action_items, impact_start, impact_end,
		alerts, state, created_by, updated_by, published_by, published_at, created_at, updated_at
		FROM incident_postmortems WHERE incident_id = ?`, incidentID).Scan(&p.IncidentID, &p.Summary, &p.Impact, &p.RootCause, &p.Resolution,
		&items, &start, &end, &alerts, &p.State, &p.CreatedBy, &p.UpdatedBy, &p.PublishedBy, &pubAt, &created, &upd)
	if errors.Is(err, sql.ErrNoRows) {
		return IncidentPostmortem{}, ErrIncidentPostmortemNotFound
	}
	if err != nil {
		return IncidentPostmortem{}, fmt.Errorf("store: get incident postmortem: %w", err)
	}
	if err := json.Unmarshal([]byte(items), &p.ActionItems); err != nil {
		return IncidentPostmortem{}, fmt.Errorf("store: decode postmortem action items: %w", err)
	}
	if err := json.Unmarshal([]byte(alerts), &p.Alerts); err != nil {
		return IncidentPostmortem{}, fmt.Errorf("store: decode postmortem alerts: %w", err)
	}
	if p.ImpactStart, err = parseStatusTime(start); err != nil {
		return IncidentPostmortem{}, err
	}
	if p.ImpactEnd, err = parseNullableStatusTime(end); err != nil {
		return IncidentPostmortem{}, err
	}
	if p.PublishedAt, err = parseNullableStatusTime(pubAt); err != nil {
		return IncidentPostmortem{}, err
	}
	if p.CreatedAt, err = parseStatusTime(created); err != nil {
		return IncidentPostmortem{}, err
	}
	if p.UpdatedAt, err = parseStatusTime(upd); err != nil {
		return IncidentPostmortem{}, err
	}
	return p, nil
}

// DraftIncidentPostmortem creates the draft or refreshes only its derived fields, so a re-draft
// never overwrites text a concurrent edit just saved. A published row is refused.
func (db *DB) DraftIncidentPostmortem(ctx context.Context, p IncidentPostmortem) error {
	alerts, err := json.Marshal(nonNilList(p.Alerts))
	if err != nil {
		return fmt.Errorf("store: encode postmortem alerts: %w", err)
	}
	var end any
	if p.ImpactEnd != nil {
		end = statusTime(*p.ImpactEnd)
	}
	now := statusTime(p.UpdatedAt)
	res, err := db.ExecContext(ctx, `INSERT INTO incident_postmortems (incident_id, impact_start, impact_end, alerts, state, created_by, updated_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'draft', ?, ?, ?, ?)
		ON CONFLICT (incident_id) DO UPDATE SET impact_start = excluded.impact_start, impact_end = excluded.impact_end,
			alerts = excluded.alerts, updated_by = excluded.updated_by, updated_at = excluded.updated_at
		WHERE incident_postmortems.state = 'draft'`,
		p.IncidentID, statusTime(p.ImpactStart), end, string(alerts), p.UpdatedBy, p.UpdatedBy, now, now)
	if err != nil {
		return fmt.Errorf("store: draft incident postmortem: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrIncidentPostmortemPublished
	}
	return nil
}

// PostmortemEdit names the text fields one write changes; nil keeps the stored value.
type PostmortemEdit struct {
	Summary     *string
	Impact      *string
	RootCause   *string
	Resolution  *string
	ActionItems *[]string
}

// EditIncidentPostmortem sets only the provided columns of a draft, so two editors of
// different fields never undo each other.
func (db *DB) EditIncidentPostmortem(ctx context.Context, incidentID string, e PostmortemEdit, by string, at time.Time) error {
	sets := []string{"updated_by = ?", "updated_at = ?"}
	args := []any{by, statusTime(at)}
	for _, f := range []struct {
		col string
		v   *string
	}{{"summary", e.Summary}, {"impact", e.Impact}, {"root_cause", e.RootCause}, {"resolution", e.Resolution}} {
		if f.v != nil {
			sets = append(sets, f.col+" = ?")
			args = append(args, *f.v)
		}
	}
	if e.ActionItems != nil {
		items, err := json.Marshal(nonNilList(*e.ActionItems))
		if err != nil {
			return fmt.Errorf("store: encode postmortem action items: %w", err)
		}
		sets = append(sets, "action_items = ?")
		args = append(args, string(items))
	}
	args = append(args, incidentID)
	//nolint:gosec // column names come from the fixed list above, values are bound
	res, err := db.ExecContext(ctx, `UPDATE incident_postmortems SET `+strings.Join(sets, ", ")+` WHERE incident_id = ? AND state = 'draft'`, args...)
	if err != nil {
		return fmt.Errorf("store: edit incident postmortem: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		if _, gerr := db.GetIncidentPostmortem(ctx, incidentID); gerr != nil {
			return gerr
		}
		return ErrIncidentPostmortemPublished
	}
	return nil
}

// PublishIncidentPostmortem flips a complete draft to published in one conditional update, so
// only one caller wins and a concurrent edit cannot empty a field between check and publish.
func (db *DB) PublishIncidentPostmortem(ctx context.Context, incidentID, by string, at time.Time) (IncidentPostmortem, error) {
	ts := statusTime(at)
	res, err := db.ExecContext(ctx, `UPDATE incident_postmortems SET state = 'published', published_by = ?, published_at = ?, updated_at = ?
		WHERE incident_id = ? AND state = 'draft' AND summary <> '' AND root_cause <> ''`, by, ts, ts, incidentID)
	if err != nil {
		return IncidentPostmortem{}, fmt.Errorf("store: publish incident postmortem: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		cur, gerr := db.GetIncidentPostmortem(ctx, incidentID)
		switch {
		case gerr != nil:
			return IncidentPostmortem{}, gerr
		case cur.State == PostmortemStatePublished:
			return IncidentPostmortem{}, ErrIncidentPostmortemPublished
		default:
			return IncidentPostmortem{}, ErrIncidentPostmortemIncomplete
		}
	}
	return db.GetIncidentPostmortem(ctx, incidentID)
}

// publishedPostmortemIDs is the subquery that keeps a reviewed incident from retention and delete.
const publishedPostmortemIDs = `SELECT incident_id FROM incident_postmortems WHERE state = 'published'`
