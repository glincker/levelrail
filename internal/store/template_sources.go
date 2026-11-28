package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrTemplateSourceNotFound marks an unknown template source name.
var ErrTemplateSourceNotFound = errors.New("store: template source not found")

// ErrTemplateSourceExists marks a name already taken.
var ErrTemplateSourceExists = errors.New("store: template source already exists")

// TemplateSource is one git repo of templates. It never carries key material:
// DeployKeyApp only names the app whose stored deploy key is reused.
type TemplateSource struct {
	Name         string `json:"name"`
	GitURL       string `json:"git_url"`
	Ref          string `json:"ref,omitempty"`
	DeployKeyApp string `json:"deploy_key_app,omitempty"`
	Status       string `json:"status"`
	StatusDetail string `json:"status_detail,omitempty"`
	LastSync     string `json:"last_sync,omitempty"`
	CreatedAt    string `json:"created_at"`
	CommitSHA    string `json:"commit_sha,omitempty"`
	Templates    int    `json:"templates"`
}

// TemplateSourceEntry is one validated template of a source.
type TemplateSourceEntry struct {
	Source       string `json:"source"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	ContentHash  string `json:"content_hash,omitempty"`
	Review       string `json:"review,omitempty"`
	TemplateYAML string `json:"-"`
	ComposeYAML  string `json:"-"`
}

// Review states of a source entry. A changed entry that was approved before drops to needs_review.
const (
	TemplateReviewApproved    = "approved"
	TemplateReviewNeedsReview = "needs_review"
)

// CreateTemplateSource inserts a source in pending state.
func (db *DB) CreateTemplateSource(ctx context.Context, s TemplateSource) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO template_sources (name, git_url, ref, deploy_key_app, status, created_at)
		VALUES (?, ?, ?, ?, 'pending', ?)
	`, s.Name, s.GitURL, s.Ref, s.DeployKeyApp, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "PRIMARY KEY") {
			return ErrTemplateSourceExists
		}
		return fmt.Errorf("store: create template source %q: %w", s.Name, err)
	}
	return nil
}

// GetTemplateSource returns one source with its entry count.
func (db *DB) GetTemplateSource(ctx context.Context, name string) (TemplateSource, error) {
	list, err := db.listTemplateSources(ctx, `WHERE s.name = ?`, name)
	if err != nil {
		return TemplateSource{}, err
	}
	if len(list) == 0 {
		return TemplateSource{}, ErrTemplateSourceNotFound
	}
	return list[0], nil
}

// ListTemplateSources returns every source by name.
func (db *DB) ListTemplateSources(ctx context.Context) ([]TemplateSource, error) {
	return db.listTemplateSources(ctx, ``)
}

func (db *DB) listTemplateSources(ctx context.Context, where string, args ...any) ([]TemplateSource, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.name, s.git_url, s.ref, s.deploy_key_app, s.status, s.status_detail, s.last_sync, s.created_at, s.commit_sha,
		       (SELECT COUNT(*) FROM template_source_entries e WHERE e.source = s.name)
		FROM template_sources s `+where+` ORDER BY s.name`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list template sources: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TemplateSource
	for rows.Next() {
		var s TemplateSource
		if err := rows.Scan(&s.Name, &s.GitURL, &s.Ref, &s.DeployKeyApp, &s.Status, &s.StatusDetail, &s.LastSync, &s.CreatedAt, &s.CommitSHA, &s.Templates); err != nil {
			return nil, fmt.Errorf("store: scan template source: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteTemplateSource removes a source and its entries.
func (db *DB) DeleteTemplateSource(ctx context.Context, name string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete template source: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM template_source_entries WHERE source = ?`, name); err != nil {
		return fmt.Errorf("store: delete template source entries: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM template_source_approvals WHERE source = ?`, name); err != nil {
		return fmt.Errorf("store: delete template source approvals: %w", err)
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM template_sources WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("store: delete template source: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTemplateSourceNotFound
	}
	return tx.Commit()
}

// ReplaceTemplateSourceEntries swaps a source's entries for the validated set of a sync and
// records the commit. An entry is approved only when root approved the same content hash before.
func (db *DB) ReplaceTemplateSourceEntries(ctx context.Context, name, commit string, entries []TemplateSourceEntry) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: replace template source entries: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	approved := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT id, content_hash FROM template_source_approvals WHERE source = ?`, name)
	if err != nil {
		return fmt.Errorf("store: read template source approvals: %w", err)
	}
	for rows.Next() {
		var id, h string
		if err := rows.Scan(&id, &h); err != nil {
			_ = rows.Close()
			return fmt.Errorf("store: scan template source approval: %w", err)
		}
		approved[id] = h
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("store: read template source approvals: %w", err)
	}
	_ = rows.Close()
	if _, err := tx.ExecContext(ctx, `DELETE FROM template_source_entries WHERE source = ?`, name); err != nil {
		return fmt.Errorf("store: clear template source entries: %w", err)
	}
	for _, e := range entries {
		// Only a hash that root approved before stays approved; every other id needs review.
		review := TemplateReviewNeedsReview
		if h, ok := approved[e.ID]; ok && h == e.ContentHash && h != "" {
			review = TemplateReviewApproved
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO template_source_entries (source, id, name, template_yaml, compose_yaml, content_hash, review) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			name, e.ID, e.Name, e.TemplateYAML, e.ComposeYAML, e.ContentHash, review); err != nil {
			return fmt.Errorf("store: insert template source entry %q: %w", e.ID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE template_sources SET commit_sha = ? WHERE name = ?`, commit, name); err != nil {
		return fmt.Errorf("store: record template source commit: %w", err)
	}
	return tx.Commit()
}

// ErrTemplateSourceHashMismatch marks an approval for content that is no longer the current entry.
var ErrTemplateSourceHashMismatch = errors.New("store: template source entry changed since it was reviewed")

// ApproveTemplateSourceEntry marks the reviewed content usable and remembers its hash. It only
// matches the entry whose content hash equals hash.
func (db *DB) ApproveTemplateSourceEntry(ctx context.Context, source, id, hash string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: approve template source entry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, `UPDATE template_source_entries SET review = ? WHERE source = ? AND id = ? AND content_hash = ? AND content_hash != ''`, TemplateReviewApproved, source, id, hash)
	if err != nil {
		return fmt.Errorf("store: approve template source entry: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		var one int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM template_source_entries WHERE source = ? AND id = ?`, source, id).Scan(&one); err != nil {
			return ErrTemplateSourceNotFound
		}
		return ErrTemplateSourceHashMismatch
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO template_source_approvals (source, id, content_hash) VALUES (?, ?, ?)
		ON CONFLICT(source, id) DO UPDATE SET content_hash = excluded.content_hash`, source, id, hash); err != nil {
		return fmt.Errorf("store: record template source approval: %w", err)
	}
	return tx.Commit()
}

// SetTemplateSourceStatus records the outcome of a sync attempt.
func (db *DB) SetTemplateSourceStatus(ctx context.Context, name, status, detail string) error {
	res, err := db.ExecContext(ctx, `UPDATE template_sources SET status = ?, status_detail = ?, last_sync = ? WHERE name = ?`,
		status, detail, time.Now().UTC().Format(time.RFC3339), name)
	if err != nil {
		return fmt.Errorf("store: set template source status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrTemplateSourceNotFound
	}
	return nil
}

// ListTemplateSourceEntries returns the entries of one source, or of all when name is empty.
func (db *DB) ListTemplateSourceEntries(ctx context.Context, name string) ([]TemplateSourceEntry, error) {
	q, args := `SELECT source, id, name, content_hash, review, template_yaml, compose_yaml FROM template_source_entries ORDER BY source, id`, []any(nil)
	if name != "" {
		q, args = `SELECT source, id, name, content_hash, review, template_yaml, compose_yaml FROM template_source_entries WHERE source = ? ORDER BY id`, []any{name}
	}
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list template source entries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TemplateSourceEntry
	for rows.Next() {
		var e TemplateSourceEntry
		if err := rows.Scan(&e.Source, &e.ID, &e.Name, &e.ContentHash, &e.Review, &e.TemplateYAML, &e.ComposeYAML); err != nil {
			return nil, fmt.Errorf("store: scan template source entry: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
