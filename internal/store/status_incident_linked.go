package store

import (
	"context"
	"fmt"
)

// SourceMonitorStatus marks public incidents opened for monitors that opted in to status page linking.
const SourceMonitorStatus = "monitor_status"

// ListOpenLinkedIncidents returns unresolved incidents the monitor link opened, one per monitor outage.
func (db *DB) ListOpenLinkedIncidents(ctx context.Context) ([]InternalStatusIncident, error) {
	rows, err := db.QueryContext(ctx, internalIncidentColumns+` WHERE source = ? AND internal_state != 'resolved' ORDER BY starts_at DESC`, SourceMonitorStatus)
	if err != nil {
		return nil, fmt.Errorf("store: list linked incidents: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []InternalStatusIncident
	for rows.Next() {
		i, err := scanInternalIncident(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan linked incident: %w", err)
		}
		out = append(out, i)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate linked incidents: %w", err)
	}
	return out, nil
}
