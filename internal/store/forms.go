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

var (
	// ErrFormNotFound is returned when a form id or app/name pair has no row.
	ErrFormNotFound = errors.New("store: form not found")
	// ErrFormExists is returned when an app already has a form of that name.
	ErrFormExists = errors.New("store: form already exists")
	// ErrFormSubmissionNotFound is returned when a submission is not in the given form.
	ErrFormSubmissionNotFound = errors.New("store: form submission not found")
)

// Form is one capture endpoint (migrations/2560_forms.sql).
type Form struct {
	ID               string
	AppName          string
	Name             string
	RedirectURL      string
	HoneypotField    string
	NotifyChannelIDs []string
	// IPSalt keys the submitter hash so hashes cannot be matched across forms.
	IPSalt    string
	CreatedAt time.Time
	// Schema is the designer JSON; "{}" means the default fields.
	Schema string
}

// FormSubmission is one captured post; Fields holds string values only.
type FormSubmission struct {
	ID         int64
	FormID     string
	Fields     map[string]string
	Spam       bool
	SpamReason string
	IPHash     string
	CreatedAt  time.Time
	// Test marks a submission sent from the dashboard test button.
	Test bool
}

// FormSubmissionQuery filters a submission listing; Spam nil returns both.
type FormSubmissionQuery struct {
	Search string
	Spam   *bool
	Limit  int
	// BeforeID returns only rows older than this id (keyset paging); 0 starts at the newest.
	BeforeID int64
}

// FormCounts is a form's stored submission totals.
type FormCounts struct {
	Total int
	Spam  int
}

const formColumns = `id, app_name, name, redirect_url, honeypot_field, notify_channel_ids, ip_salt, created_at, schema`

func schemaOrEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "{}"
	}
	return s
}

func scanForm(scan func(dest ...any) error) (Form, error) {
	var f Form
	var channels, created string
	if err := scan(&f.ID, &f.AppName, &f.Name, &f.RedirectURL, &f.HoneypotField, &channels, &f.IPSalt, &created, &f.Schema); err != nil {
		return Form{}, err
	}
	if err := json.Unmarshal([]byte(channels), &f.NotifyChannelIDs); err != nil {
		return Form{}, fmt.Errorf("notify_channel_ids: %w", err)
	}
	t, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return Form{}, fmt.Errorf("created_at: %w", err)
	}
	f.CreatedAt = t
	return f, nil
}

// CreateForm inserts f, returning ErrFormExists when the app already has that name.
func (db *DB) CreateForm(ctx context.Context, f Form) error {
	channels, err := json.Marshal(nonNilStrings(f.NotifyChannelIDs))
	if err != nil {
		return fmt.Errorf("store: create form %q: %w", f.Name, err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO forms (`+formColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		f.ID, f.AppName, f.Name, f.RedirectURL, f.HoneypotField, string(channels), f.IPSalt, f.CreatedAt.UTC().Format(time.RFC3339Nano), schemaOrEmpty(f.Schema))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ErrFormExists
		}
		return fmt.Errorf("store: create form %q: %w", f.Name, err)
	}
	return nil
}

// UpdateForm replaces a form's redirect, honeypot and notification channels.
func (db *DB) UpdateForm(ctx context.Context, f Form) error {
	channels, err := json.Marshal(nonNilStrings(f.NotifyChannelIDs))
	if err != nil {
		return fmt.Errorf("store: update form %q: %w", f.ID, err)
	}
	res, err := db.ExecContext(ctx, `UPDATE forms SET redirect_url = ?, honeypot_field = ?, notify_channel_ids = ?, schema = ? WHERE id = ?`,
		f.RedirectURL, f.HoneypotField, string(channels), schemaOrEmpty(f.Schema), f.ID)
	if err != nil {
		return fmt.Errorf("store: update form %q: %w", f.ID, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFormNotFound
	}
	return nil
}

// GetForm returns the form named name in app.
func (db *DB) GetForm(ctx context.Context, app, name string) (Form, error) {
	f, err := scanForm(db.QueryRowContext(ctx, `SELECT `+formColumns+` FROM forms WHERE app_name = ? AND name = ?`, app, name).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return Form{}, ErrFormNotFound
	}
	if err != nil {
		return Form{}, fmt.Errorf("store: get form %q/%q: %w", app, name, err)
	}
	return f, nil
}

// ListForms returns an app's forms with their submission counts, ordered by name.
func (db *DB) ListForms(ctx context.Context, app string) ([]Form, map[string]FormCounts, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+formColumns+` FROM forms WHERE app_name = ? ORDER BY name`, app)
	if err != nil {
		return nil, nil, fmt.Errorf("store: list forms for %q: %w", app, err)
	}
	var out []Form
	for rows.Next() {
		f, err := scanForm(rows.Scan)
		if err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("store: list forms for %q: %w", app, err)
		}
		out = append(out, f)
	}
	if err := rows.Close(); err != nil {
		return nil, nil, fmt.Errorf("store: list forms for %q: %w", app, err)
	}
	counts := map[string]FormCounts{}
	crow, err := db.QueryContext(ctx, `SELECT s.form_id, COUNT(*), COALESCE(SUM(s.spam), 0)
		FROM form_submissions s JOIN forms f ON f.id = s.form_id WHERE f.app_name = ? GROUP BY s.form_id`, app)
	if err != nil {
		return nil, nil, fmt.Errorf("store: count form submissions for %q: %w", app, err)
	}
	defer func() { _ = crow.Close() }()
	for crow.Next() {
		var id string
		var c FormCounts
		if err := crow.Scan(&id, &c.Total, &c.Spam); err != nil {
			return nil, nil, fmt.Errorf("store: count form submissions for %q: %w", app, err)
		}
		counts[id] = c
	}
	return out, counts, crow.Err()
}

// DeleteForm removes a form and, through the cascade, every submission it holds.
func (db *DB) DeleteForm(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM forms WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete form %q: %w", id, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrFormNotFound
	}
	return nil
}

// DeleteAppForms removes every form of an app and its submissions.
func (db *DB) DeleteAppForms(ctx context.Context, app string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM forms WHERE app_name = ?`, app); err != nil {
		return fmt.Errorf("store: delete forms for %q: %w", app, err)
	}
	return nil
}
