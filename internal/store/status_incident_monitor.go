package store

import (
	"context"
	"database/sql"
	"encoding/json"
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

// AddStatusIncidentComponents merges component ids into an incident so a monitor folded into it
// shows the components it affects. Existing ids keep their order and nothing is duplicated.
func (db *DB) AddStatusIncidentComponents(ctx context.Context, incidentID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: merge incident components: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT component_ids FROM status_incidents WHERE id = ?`, incidentID).Scan(&raw); err != nil {
		return fmt.Errorf("store: read incident components %q: %w", incidentID, err)
	}
	var have []string
	_ = json.Unmarshal([]byte(raw), &have)
	seen := make(map[string]bool, len(have))
	for _, id := range have {
		seen[id] = true
	}
	changed := false
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			have = append(have, id)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	b, err := json.Marshal(have)
	if err != nil {
		return fmt.Errorf("store: encode incident components: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE status_incidents SET component_ids = ?, updated_at = ? WHERE id = ?`, string(b), statusTime(time.Now()), incidentID); err != nil {
		return fmt.Errorf("store: merge incident components %q: %w", incidentID, err)
	}
	return tx.Commit()
}
