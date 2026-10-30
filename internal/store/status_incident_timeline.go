package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// IncidentTimelineEntry is one timeline row with its visibility and author, for operator views.
type IncidentTimelineEntry struct {
	ID         string
	IncidentID string
	Status     string
	Body       string
	Visibility string
	Author     string
	CreatedAt  time.Time
}

// ListIncidentTimeline returns every entry of one incident, internal ones included, oldest first.
func (db *DB) ListIncidentTimeline(ctx context.Context, incidentID string) ([]IncidentTimelineEntry, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, incident_id, status, body, visibility, author, created_at
		FROM status_incident_updates WHERE incident_id = ? ORDER BY created_at`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("store: list incident timeline: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []IncidentTimelineEntry
	for rows.Next() {
		var (
			e      IncidentTimelineEntry
			author sql.NullString
			at     string
		)
		if err := rows.Scan(&e.ID, &e.IncidentID, &e.Status, &e.Body, &e.Visibility, &author, &at); err != nil {
			return nil, fmt.Errorf("store: scan incident timeline: %w", err)
		}
		e.Author = author.String
		if e.CreatedAt, err = parseStatusTime(at); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate incident timeline: %w", err)
	}
	return out, nil
}
