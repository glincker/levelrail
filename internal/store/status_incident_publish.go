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

// Publish refusals, mapped to HTTP statuses by the API layer.
var (
	ErrIncidentPublished  = errors.New("store: incident is already published")
	ErrIncidentStale      = errors.New("store: incident changed since it was loaded")
	ErrIncidentDraftEmpty = errors.New("store: incident draft is empty")
)

// SaveStatusIncidentDraftFields stores the unpublished draft; public reads cannot see an unpublished row.
func (db *DB) SaveStatusIncidentDraftFields(ctx context.Context, id, title, status, body, impact string, componentIDs []string) error {
	comps, err := json.Marshal(nonNilIDs(componentIDs))
	if err != nil {
		return fmt.Errorf("store: encode incident components: %w", err)
	}
	res, err := db.ExecContext(ctx, `UPDATE status_incidents SET draft_title = ?, draft_body = ?, status = ?, impact = ?, component_ids = ?, updated_at = ? WHERE id = ? AND published = 0`,
		nullableText(title), nullableText(body), status, impact, string(comps), statusTime(time.Now()), id)
	if err != nil {
		return fmt.Errorf("store: save incident draft %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		if _, gerr := db.GetInternalStatusIncident(ctx, id); gerr == nil {
			return ErrIncidentPublished
		}
		return ErrStatusIncidentNotFound
	}
	return nil
}

// PublishStatusIncident copies the draft into the public title and a first public update in one
// transaction. seenUpdatedAt must match the row so a stale form cannot publish over a newer draft.
func (db *DB) PublishStatusIncident(ctx context.Context, id string, seenUpdatedAt time.Time, by, updateID string, at time.Time) (InternalStatusIncident, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return InternalStatusIncident{}, fmt.Errorf("store: publish incident %q: %w", id, err)
	}
	defer func() { _ = tx.Rollback() }()
	inc, err := scanInternalIncident(tx.QueryRowContext(ctx, internalIncidentColumns+` WHERE id = ?`, id).Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return InternalStatusIncident{}, ErrStatusIncidentNotFound
		}
		return InternalStatusIncident{}, fmt.Errorf("store: publish incident %q: %w", id, err)
	}
	if inc.Published {
		return InternalStatusIncident{}, ErrIncidentPublished
	}
	if !inc.UpdatedAt.Equal(seenUpdatedAt) {
		return InternalStatusIncident{}, ErrIncidentStale
	}
	title, body := strings.TrimSpace(inc.DraftTitle), strings.TrimSpace(inc.DraftBody)
	if title == "" || body == "" {
		return InternalStatusIncident{}, ErrIncidentDraftEmpty
	}
	ts := statusTime(at)
	var resolved any
	if inc.Status == "resolved" || inc.Status == "completed" {
		resolved = ts
	}
	if _, err := tx.ExecContext(ctx, `UPDATE status_incidents SET title = ?, published = 1, published_at = ?, published_by = ?, resolved_at = ?, updated_at = ? WHERE id = ? AND published = 0`,
		title, ts, nullableText(by), resolved, ts, id); err != nil {
		return InternalStatusIncident{}, fmt.Errorf("store: publish incident %q: %w", id, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO status_incident_updates (id, incident_id, status, body, created_at, visibility, author) VALUES (?, ?, ?, ?, ?, 'public', ?)`,
		updateID, id, inc.Status, body, ts, nullableText(by)); err != nil {
		return InternalStatusIncident{}, fmt.Errorf("store: publish incident %q: first update: %w", id, err)
	}
	if err := tx.Commit(); err != nil {
		return InternalStatusIncident{}, fmt.Errorf("store: publish incident %q: %w", id, err)
	}
	inc.Title, inc.Published, inc.PublishedBy = title, true, by
	inc.PublishedAt = &at
	return inc, nil
}
