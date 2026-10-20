package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrCustomTemplateNotFound is returned by GetCustomTemplate and
// DeleteCustomTemplate when id doesn't match any row.
var ErrCustomTemplateNotFound = errors.New("store: custom template not found")

// CustomTemplate is an operator-defined one-click template
// (migrations/0270_custom_templates.sql), built from a running app's
// desired state (internal/compose.FromDesiredServices). Compose is a
// plain compose.yaml body, same shape as catalog.Template.Compose, so
// it deploys through the built-in catalog's own routes. Never holds a
// real secret value: see FromDesiredServices.
type CustomTemplate struct {
	ID          string
	Name        string
	Description string
	Compose     string
	// SourceApp is the app name this was captured from, informational
	// only: not a foreign key, so renaming or deleting that app later
	// never affects a template already saved from it.
	SourceApp string
	// CreatedBy is the principal ID (internal/api's callerPrincipal)
	// that saved this template, empty if that lookup failed at save
	// time.
	CreatedBy string
	CreatedAt string
	UpdatedAt string
}

// SaveCustomTemplate inserts a new custom template row. IDs are minted
// by the caller (internal/api, the same "generate before the INSERT"
// pattern SaveBackupTarget's own doc comment establishes), prefixed so
// they can never collide with a internal/catalog slug (see
// internal/api/service_templates.go's resolveTemplate).
func (db *DB) SaveCustomTemplate(ctx context.Context, t CustomTemplate) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO custom_templates (id, name, description, compose, source_app, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.Name, t.Description, t.Compose, t.SourceApp, t.CreatedBy, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return fmt.Errorf("store: save custom template %q: %w", t.ID, err)
	}
	return nil
}

// GetCustomTemplate returns the custom template with this ID, or
// ErrCustomTemplateNotFound.
func (db *DB) GetCustomTemplate(ctx context.Context, id string) (CustomTemplate, error) {
	var t CustomTemplate
	err := db.QueryRowContext(ctx, `
		SELECT id, name, description, compose, source_app, created_by, created_at, updated_at
		FROM custom_templates
		WHERE id = ?
	`, id).Scan(&t.ID, &t.Name, &t.Description, &t.Compose, &t.SourceApp, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return CustomTemplate{}, ErrCustomTemplateNotFound
	}
	if err != nil {
		return CustomTemplate{}, fmt.Errorf("store: get custom template %q: %w", id, err)
	}
	return t, nil
}

// ListCustomTemplates returns every custom template, oldest first
// (creation order, the same order ListBackupTargets already uses for
// a settings-style listing).
func (db *DB) ListCustomTemplates(ctx context.Context) ([]CustomTemplate, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, description, compose, source_app, created_by, created_at, updated_at
		FROM custom_templates
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list custom templates: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []CustomTemplate
	for rows.Next() {
		var t CustomTemplate
		if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Compose, &t.SourceApp, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("store: scan custom template row: %w", err)
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate custom template rows: %w", err)
	}
	return out, nil
}

// DeleteCustomTemplate removes a custom template row. Returns
// ErrCustomTemplateNotFound if id doesn't exist.
func (db *DB) DeleteCustomTemplate(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM custom_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete custom template %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: delete custom template %q: %w", id, err)
	}
	if n == 0 {
		return ErrCustomTemplateNotFound
	}
	return nil
}
