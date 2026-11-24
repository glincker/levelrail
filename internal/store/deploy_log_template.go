package store

import (
	"context"
	"fmt"
	"time"
)

// DeployLogTemplate is one mined log cluster of a deploy. Sample must already be secret-masked.
type DeployLogTemplate struct {
	DeployID   string `json:"deploy_id"`
	TemplateID string `json:"template_id"`
	Template   string `json:"template"`
	Count      int    `json:"count"`
	Sample     string `json:"sample"`
}

// ReplaceDeployLogTemplates swaps the stored templates of one deploy in a single transaction.
// Wiring into the deploy log finish path needs design: this is the write API only.
func (db *DB) ReplaceDeployLogTemplates(ctx context.Context, deployID string, rows []DeployLogTemplate) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: begin deploy log templates: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM deploy_log_templates WHERE deploy_id = ?`, deployID); err != nil {
		return fmt.Errorf("store: clear deploy log templates %q: %w", deployID, err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, r := range rows {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO deploy_log_templates (deploy_id, template_id, template, count, sample, created_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(deploy_id, template_id) DO UPDATE SET count = count + excluded.count
		`, deployID, r.TemplateID, r.Template, r.Count, r.Sample, now); err != nil {
			return fmt.Errorf("store: save deploy log template %q: %w", r.TemplateID, err)
		}
	}
	return tx.Commit()
}

// ListDeployLogTemplates returns the templates of one deploy, most frequent first.
func (db *DB) ListDeployLogTemplates(ctx context.Context, deployID string) ([]DeployLogTemplate, error) {
	rows, err := db.reader().QueryContext(ctx, `
		SELECT deploy_id, template_id, template, count, sample FROM deploy_log_templates
		WHERE deploy_id = ? ORDER BY count DESC, template_id
	`, deployID)
	if err != nil {
		return nil, fmt.Errorf("store: list deploy log templates %q: %w", deployID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []DeployLogTemplate
	for rows.Next() {
		var r DeployLogTemplate
		if err := rows.Scan(&r.DeployID, &r.TemplateID, &r.Template, &r.Count, &r.Sample); err != nil {
			return nil, fmt.Errorf("store: scan deploy log template: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
