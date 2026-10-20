package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrAppNotFound is returned by GetApp, GetAppByName, and DeleteApp when
// no row matches.
var ErrAppNotFound = errors.New("store: app not found")

// App is stage 1 of multi-service apps (migrations/0039_apps.sql): a
// named owner that zero or more store.DesiredService rows can belong to
// via their AppID field. ProjectID mirrors DesiredService.ProjectID:
// empty string means no project, not merely "unset".
type App struct {
	ID        string
	Name      string
	ProjectID string
	CreatedAt string
	UpdatedAt string
}

// SaveApp creates or fully replaces an app row, keyed by ID. Matches
// SaveDesiredService's full-record-replace semantics, not
// SaveProject's insert-only ones: like a service, an app has an
// UpdatedAt an ordinary edit is expected to move forward.
func (db *DB) SaveApp(ctx context.Context, a App) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO apps (id, name, project_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			project_id = excluded.project_id,
			updated_at = excluded.updated_at
	`, a.ID, a.Name, sql.NullString{String: a.ProjectID, Valid: a.ProjectID != ""}, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save app %q: %w", a.ID, err)
	}
	return nil
}

// GetApp returns the app with this ID, or ErrAppNotFound.
func (db *DB) GetApp(ctx context.Context, id string) (App, error) {
	return scanApp(db.QueryRowContext(ctx, `
		SELECT id, name, project_id, created_at, updated_at
		FROM apps
		WHERE id = ?
	`, id))
}

// GetAppByName returns the app with this name, or ErrAppNotFound. A
// separate method from GetApp rather than one "id or name" lookup:
// id and name are different columns with different uniqueness
// guarantees (id is the primary key, name has its own UNIQUE
// constraint), so which one a caller has is always known at the call
// site, the same way GetProject (by id) and GetDesiredService (by
// name) are already two distinct methods rather than one overloaded
// one.
func (db *DB) GetAppByName(ctx context.Context, name string) (App, error) {
	return scanApp(db.QueryRowContext(ctx, `
		SELECT id, name, project_id, created_at, updated_at
		FROM apps
		WHERE name = ?
	`, name))
}

func scanApp(row *sql.Row) (App, error) {
	var a App
	var projectID sql.NullString
	err := row.Scan(&a.ID, &a.Name, &projectID, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return App{}, ErrAppNotFound
	}
	if err != nil {
		return App{}, fmt.Errorf("store: scan app: %w", err)
	}
	a.ProjectID = projectID.String
	return a, nil
}

// ListApps returns every app, ordered by name, the same "predictable
// listing order" convention ListDesiredServices already uses.
func (db *DB) ListApps(ctx context.Context) ([]App, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, project_id, created_at, updated_at
		FROM apps
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list apps: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []App
	for rows.Next() {
		var a App
		var projectID sql.NullString
		if err := rows.Scan(&a.ID, &a.Name, &projectID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan app row: %w", err)
		}
		a.ProjectID = projectID.String
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate app rows: %w", err)
	}
	return out, nil
}

// DeleteApp removes an app row. desired_services.app_id is
// "REFERENCES apps(id) ON DELETE CASCADE" (migrations/0039_apps.sql),
// so every service belonging to this app is deleted along with it,
// unlike DeleteProject's ON DELETE SET NULL: an app owns its services'
// lifecycle, a project is only a label. Returns ErrAppNotFound if id
// doesn't exist.
func (db *DB) DeleteApp(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete app %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete app %q: rows affected: %w", id, err)
	}
	if n == 0 {
		return ErrAppNotFound
	}
	return nil
}

// ListServicesByApp returns every service belonging to appID, ordered
// by name. This is the read side of stage 1 multi-service apps: an
// app's member services, for a group-status rollup or a detail view.
func (db *DB) ListServicesByApp(ctx context.Context, appID string) ([]DesiredService, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT `+desiredServiceColumns+`
		FROM desired_services
		WHERE app_id = ?
		ORDER BY name
	`, appID)
	if err != nil {
		return nil, fmt.Errorf("store: list services for app %q: %w", appID, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DesiredService
	for rows.Next() {
		svc, err := scanDesiredService(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan desired service row: %w", err)
		}
		out = append(out, *svc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate desired service rows: %w", err)
	}
	return out, nil
}
