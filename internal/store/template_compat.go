package store

import (
	"context"
	"fmt"
	"time"
)

// TemplateCompat is one recorded install gap for a template on a node architecture.
type TemplateCompat struct {
	TemplateID string `json:"template_id"`
	NodeArch   string `json:"node_arch"`
	Class      string `json:"class"`
	Detail     string `json:"detail,omitempty"`
	Failures   int    `json:"failures"`
	FirstSeen  string `json:"first_seen"`
	LastSeen   string `json:"last_seen"`
}

// UpsertTemplateCompat records a failed install, bumping the count when the row exists.
func (db *DB) UpsertTemplateCompat(ctx context.Context, templateID, nodeArch, class, detail string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx, `
		INSERT INTO template_compat (template_id, node_arch, class, detail, failures, first_seen, last_seen)
		VALUES (?, ?, ?, ?, 1, ?, ?)
		ON CONFLICT (template_id, node_arch, class)
		DO UPDATE SET detail = excluded.detail, failures = failures + 1, last_seen = excluded.last_seen
	`, templateID, nodeArch, class, detail, now, now)
	if err != nil {
		return fmt.Errorf("store: record template compat %q: %w", templateID, err)
	}
	return nil
}

// ClearTemplateCompat drops every gap for a template on an architecture after it installs.
func (db *DB) ClearTemplateCompat(ctx context.Context, templateID, nodeArch string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM template_compat WHERE template_id = ? AND node_arch = ?`, templateID, nodeArch); err != nil {
		return fmt.Errorf("store: clear template compat %q: %w", templateID, err)
	}
	return nil
}

// ListTemplateCompat returns every recorded gap, newest first.
func (db *DB) ListTemplateCompat(ctx context.Context) ([]TemplateCompat, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT template_id, node_arch, class, detail, failures, first_seen, last_seen
		FROM template_compat ORDER BY last_seen DESC, template_id, class
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list template compat: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []TemplateCompat
	for rows.Next() {
		var c TemplateCompat
		if err := rows.Scan(&c.TemplateID, &c.NodeArch, &c.Class, &c.Detail, &c.Failures, &c.FirstSeen, &c.LastSeen); err != nil {
			return nil, fmt.Errorf("store: scan template compat: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
