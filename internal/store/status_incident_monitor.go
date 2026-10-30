package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// StatusIncidentLink ties a monitor outage to an internal incident.
type StatusIncidentLink struct {
	IncidentID string
	MonitorID  string
	DownAt     time.Time
	UpAt       *time.Time
}

// ListMonitorIncidents returns system incidents that are open or were resolved at or after since.
// The opener uses it to find fold and reopen targets without scanning the whole table.
func (db *DB) ListMonitorIncidents(ctx context.Context, since time.Time) ([]InternalStatusIncident, error) {
	rows, err := db.QueryContext(ctx, internalIncidentColumns+` WHERE source = 'monitor'
		AND (internal_state != 'resolved' OR internal_resolved_at >= ?) ORDER BY starts_at DESC`, statusTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: list monitor incidents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []InternalStatusIncident
	for rows.Next() {
		i, err := scanInternalIncident(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan monitor incident: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate monitor incidents: %w", err)
	}
	return out, nil
}

// ListStatusIncidentLinks returns every monitor link of one incident, oldest first.
func (db *DB) ListStatusIncidentLinks(ctx context.Context, incidentID string) ([]StatusIncidentLink, error) {
	rows, err := db.QueryContext(ctx, `SELECT incident_id, monitor_id, down_at, up_at FROM status_incident_links WHERE incident_id = ? ORDER BY down_at`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("store: list incident links: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StatusIncidentLink
	for rows.Next() {
		var (
			l        StatusIncidentLink
			down, up sql.NullString
		)
		if err := rows.Scan(&l.IncidentID, &l.MonitorID, &down, &up); err != nil {
			return nil, fmt.Errorf("store: scan incident link: %w", err)
		}
		if l.DownAt, err = parseStatusTime(down.String); err != nil {
			return nil, err
		}
		if l.UpAt, err = parseNullableStatusTime(up); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate incident links: %w", err)
	}
	return out, nil
}
