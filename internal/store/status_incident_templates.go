package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// StatusIncidentTemplate is a saved incident post preset.
type StatusIncidentTemplate struct {
	ID        string
	Name      string
	Title     string
	Status    string
	Impact    string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Sentinel errors for incident templates.
var (
	ErrStatusTemplateNotFound = errors.New("store: status incident template not found")
	ErrStatusTemplateName     = errors.New("store: status incident template name already used")
)

const statusTemplateColumns = `SELECT id, name, title, status, impact, body, created_at, updated_at FROM status_incident_templates`

func scanStatusTemplate(scan func(dest ...any) error) (StatusIncidentTemplate, error) {
	var (
		t            StatusIncidentTemplate
		created, upd string
	)
	if err := scan(&t.ID, &t.Name, &t.Title, &t.Status, &t.Impact, &t.Body, &created, &upd); err != nil {
		return StatusIncidentTemplate{}, err
	}
	var err error
	if t.CreatedAt, err = parseStatusTime(created); err != nil {
		return StatusIncidentTemplate{}, err
	}
	if t.UpdatedAt, err = parseStatusTime(upd); err != nil {
		return StatusIncidentTemplate{}, err
	}
	return t, nil
}

// ListStatusIncidentTemplates returns every template ordered by name.
func (db *DB) ListStatusIncidentTemplates(ctx context.Context) ([]StatusIncidentTemplate, error) {
	rows, err := db.QueryContext(ctx, statusTemplateColumns+` ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("store: list status incident templates: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StatusIncidentTemplate
	for rows.Next() {
		t, err := scanStatusTemplate(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan status incident template: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate status incident templates: %w", err)
	}
	return out, nil
}

// GetStatusIncidentTemplate returns one template or ErrStatusTemplateNotFound.
func (db *DB) GetStatusIncidentTemplate(ctx context.Context, id string) (StatusIncidentTemplate, error) {
	t, err := scanStatusTemplate(db.QueryRowContext(ctx, statusTemplateColumns+` WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return StatusIncidentTemplate{}, ErrStatusTemplateNotFound
	}
	if err != nil {
		return StatusIncidentTemplate{}, fmt.Errorf("store: get status incident template %q: %w", id, err)
	}
	return t, nil
}

// SaveStatusIncidentTemplate creates or replaces a template; a taken name returns ErrStatusTemplateName.
func (db *DB) SaveStatusIncidentTemplate(ctx context.Context, t StatusIncidentTemplate) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.ExecContext(ctx, `
		INSERT INTO status_incident_templates (id, name, title, status, impact, body, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET name = excluded.name, title = excluded.title, status = excluded.status,
			impact = excluded.impact, body = excluded.body, updated_at = excluded.updated_at`,
		t.ID, t.Name, t.Title, t.Status, t.Impact, t.Body, now, now)
	if isUniqueViolation(err) {
		return ErrStatusTemplateName
	}
	if err != nil {
		return fmt.Errorf("store: save status incident template %q: %w", t.ID, err)
	}
	return nil
}

// DeleteStatusIncidentTemplate removes a template; a missing id returns ErrStatusTemplateNotFound.
func (db *DB) DeleteStatusIncidentTemplate(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM status_incident_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete status incident template %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStatusTemplateNotFound
	}
	return nil
}
