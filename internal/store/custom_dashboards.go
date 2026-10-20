package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Errors returned by the custom dashboard queries.
var (
	ErrDashboardNotFound  = errors.New("store: dashboard not found")
	ErrDashboardNameTaken = errors.New("store: dashboard name already used")
)

// CustomDashboard is a saved dashboard; Body holds the widget document JSON.
type CustomDashboard struct {
	ID        string
	Owner     string
	Scope     string
	ProjectID string
	IsDefault bool
	Name      string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

const customDashboardCols = `id, owner, scope, project_id, is_default, name, body, created_at, updated_at`

// CreateCustomDashboard inserts a dashboard; a default replaces any earlier default of its project.
func (db *DB) CreateCustomDashboard(ctx context.Context, d CustomDashboard) error {
	return db.writeCustomDashboard(ctx, d, false)
}

// UpdateCustomDashboard replaces a dashboard's fields in place.
func (db *DB) UpdateCustomDashboard(ctx context.Context, d CustomDashboard) error {
	return db.writeCustomDashboard(ctx, d, true)
}

func (db *DB) writeCustomDashboard(ctx context.Context, d CustomDashboard, update bool) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin dashboard write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if d.IsDefault {
		if _, err := tx.ExecContext(ctx, `UPDATE custom_dashboards SET is_default = 0 WHERE project_id = ? AND id <> ?`, d.ProjectID, d.ID); err != nil {
			return fmt.Errorf("store: clear dashboard default: %w", err)
		}
	}
	if update {
		res, err := tx.ExecContext(ctx, `UPDATE custom_dashboards SET scope = ?, project_id = ?, is_default = ?, name = ?, body = ?, updated_at = ? WHERE id = ?`,
			d.Scope, d.ProjectID, d.IsDefault, d.Name, d.Body, formatTime(d.UpdatedAt), d.ID)
		if err != nil {
			return dashboardWriteErr(err, d.Name)
		}
		if err := rowsAffectedOrNotFound(res, ErrDashboardNotFound, "update dashboard %q", d.ID); err != nil {
			return err
		}
	} else {
		_, err := tx.ExecContext(ctx, `INSERT INTO custom_dashboards (`+customDashboardCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.ID, d.Owner, d.Scope, d.ProjectID, d.IsDefault, d.Name, d.Body, formatTime(d.CreatedAt), formatTime(d.UpdatedAt))
		if err != nil {
			return dashboardWriteErr(err, d.Name)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: commit dashboard write: %w", err)
	}
	return nil
}

func dashboardWriteErr(err error, name string) error {
	if isUniqueViolation(err) {
		return ErrDashboardNameTaken
	}
	return fmt.Errorf("store: save dashboard %q: %w", name, err)
}

func scanCustomDashboard(sc interface{ Scan(...any) error }) (CustomDashboard, error) {
	var d CustomDashboard
	var created, updated string
	if err := sc.Scan(&d.ID, &d.Owner, &d.Scope, &d.ProjectID, &d.IsDefault, &d.Name, &d.Body, &created, &updated); err != nil {
		return CustomDashboard{}, err
	}
	var err error
	if d.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return CustomDashboard{}, fmt.Errorf("parse dashboard created_at: %w", err)
	}
	if d.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return CustomDashboard{}, fmt.Errorf("parse dashboard updated_at: %w", err)
	}
	return d, nil
}

// GetCustomDashboard returns one dashboard by id.
func (db *DB) GetCustomDashboard(ctx context.Context, id string) (CustomDashboard, error) {
	d, err := scanCustomDashboard(db.QueryRowContext(ctx, `SELECT `+customDashboardCols+` FROM custom_dashboards WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return CustomDashboard{}, ErrDashboardNotFound
	}
	if err != nil {
		return CustomDashboard{}, fmt.Errorf("store: get dashboard %q: %w", id, err)
	}
	return d, nil
}

// ListCustomDashboards returns the dashboards the owner can see: their own plus every shared one.
// A non-empty projectID narrows to that project's dashboards.
func (db *DB) ListCustomDashboards(ctx context.Context, owner, projectID string) ([]CustomDashboard, error) {
	q := `SELECT ` + customDashboardCols + ` FROM custom_dashboards WHERE (scope = 'shared' OR owner = ?)`
	args := []any{owner}
	if projectID != "" {
		q += ` AND project_id = ?`
		args = append(args, projectID)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY is_default DESC, name`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list dashboards: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CustomDashboard
	for rows.Next() {
		d, err := scanCustomDashboard(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan dashboard: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate dashboards: %w", err)
	}
	return out, nil
}

// DeleteCustomDashboard removes a dashboard by id.
func (db *DB) DeleteCustomDashboard(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM custom_dashboards WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete dashboard %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrDashboardNotFound, "delete dashboard %q", id)
}
