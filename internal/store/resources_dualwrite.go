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

const appControllerPrefix = "application/"

// AppSnapshot decodes an app change object back into the service it was taken from.
func AppSnapshot(obj json.RawMessage) (DesiredService, error) {
	var svc DesiredService
	if err := json.Unmarshal(obj, &svc); err != nil {
		return DesiredService{}, fmt.Errorf("store: decode app snapshot: %w", err)
	}
	return svc, nil
}

// appSnapshotJSON drops every value that could be secret: env and preview values
// and inline file contents. SecretEnv only ever holds names.
func appSnapshotJSON(svc DesiredService) ([]byte, error) {
	if len(svc.Env) > 0 {
		masked := make(map[string]string, len(svc.Env))
		for k := range svc.Env {
			masked[k] = ""
		}
		svc.Env = masked
	}
	if len(svc.PreviewEnvOverrides) > 0 {
		masked := make(map[string]string, len(svc.PreviewEnvOverrides))
		for k := range svc.PreviewEnvOverrides {
			masked[k] = ""
		}
		svc.PreviewEnvOverrides = masked
	}
	if len(svc.Files) > 0 {
		files := make([]ServiceFile, len(svc.Files))
		for i, f := range svc.Files {
			f.Content = ""
			files[i] = f
		}
		svc.Files = files
	}
	return json.Marshal(svc)
}

// recordAppChange snapshots app name as q currently sees it and appends the
// matching ADDED, MODIFIED or DELETED change, inside q's transaction when q is one.
func recordAppChange(ctx context.Context, q resourceQuerier, name string) error {
	prev, prevOp, err := latestObjectVersion(ctx, q, ResourceKindApp, name)
	if err != nil {
		return err
	}
	svc, err := scanDesiredService(q.QueryRowContext(ctx, `SELECT `+desiredServiceColumns+` FROM desired_services WHERE name = ?`, name).Scan)
	c := ResourceChange{Kind: ResourceKindApp, Name: name}
	switch {
	case errors.Is(err, sql.ErrNoRows) || errors.Is(err, ErrServiceNotFound):
		if prev == 0 || prevOp == ResourceOpDeleted {
			return nil
		}
		c.Op, c.Object = ResourceOpDeleted, lastAppObject(ctx, q, prev)
	case err != nil:
		return fmt.Errorf("store: record app %q change: %w", name, err)
	default:
		c.Op, c.Namespace = ResourceOpModified, svc.ProjectID
		if prev == 0 || prevOp == ResourceOpDeleted {
			c.Op = ResourceOpAdded
		}
		if c.Object, err = appSnapshotJSON(*svc); err != nil {
			return fmt.Errorf("store: record app %q change: %w", name, err)
		}
	}
	v, err := appendResourceChange(ctx, q, c)
	if err != nil {
		return err
	}
	if r := ResourceRetentionFromEnv(); v%int64(r.CompactEvery) == 0 {
		if _, err := compactResourceChanges(ctx, q, r, time.Now()); err != nil {
			return err
		}
	}
	return nil
}

func lastAppObject(ctx context.Context, q resourceQuerier, version int64) json.RawMessage {
	var obj string
	if err := q.QueryRowContext(ctx, `SELECT object FROM resource_changes WHERE version = ?`, version).Scan(&obj); err != nil {
		return nil
	}
	return json.RawMessage(obj)
}

// noteAppChange records an app change after a single-statement write; it
// returns nil so it can replace a write path's final return.
func (db *DB) noteAppChange(ctx context.Context, name string) error {
	return recordAppChange(ctx, db, name)
}

// recordAppStatusChange records a MODIFIED app change when an application
// controller's condition statuses or reasons are about to change. Messages
// are ignored: they carry timestamps and would log every reconcile pass.
func recordAppStatusChange(ctx context.Context, q resourceQuerier, controllerName, conditionsJSON string) error {
	name, ok := strings.CutPrefix(controllerName, appControllerPrefix)
	if !ok {
		return nil
	}
	var changed int
	err := q.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM json_each(?) AS n
		WHERE NOT EXISTS (
			SELECT 1 FROM reconcile_status s
			WHERE s.controller_name = ? AND s.condition_type = json_extract(n.value, '$.t')
				AND s.status IS json_extract(n.value, '$.s') AND s.reason IS json_extract(n.value, '$.r')
		)
	`, conditionsJSON, controllerName).Scan(&changed)
	if err != nil {
		return fmt.Errorf("store: compare %s conditions: %w", controllerName, err)
	}
	if changed == 0 {
		return nil
	}
	return recordAppChange(ctx, q, name)
}

// BackfillAppResourceChanges records an ADDED change for every app with no
// change history yet, so a watch from version 0 replays every existing app.
func (db *DB) BackfillAppResourceChanges(ctx context.Context) (int, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name FROM desired_services d
		WHERE NOT EXISTS (SELECT 1 FROM resource_changes c WHERE c.kind = ? AND c.name = d.name)
		ORDER BY name
	`, ResourceKindApp)
	if err != nil {
		return 0, fmt.Errorf("store: backfill app changes: %w", err)
	}
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			_ = rows.Close()
			return 0, fmt.Errorf("store: backfill app changes: scan: %w", err)
		}
		names = append(names, n)
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("store: backfill app changes: %w", err)
	}
	for _, n := range names {
		if err := recordAppChange(ctx, db, n); err != nil {
			return 0, err
		}
	}
	return len(names), nil
}
