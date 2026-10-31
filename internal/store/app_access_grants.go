package store

import (
	"context"
	"fmt"
)

// DeleteAppAccessGrants removes the access-granting rows an app owns that
// have no foreign key to it: its function KV token hash, the webhook
// triggers of its pipelines and its secret branch overrides. It returns
// the secrets namespaces the caller must erase.
func (db *DB) DeleteAppAccessGrants(ctx context.Context, name string) ([]string, error) {
	var namespaces []string

	if _, err := db.ExecContext(ctx, `DELETE FROM function_kv_tokens WHERE namespace = ?`, name); err != nil {
		return nil, fmt.Errorf("store: delete kv token of %q: %w", name, err)
	}

	ids, err := db.stringsQuery(ctx, `SELECT id FROM pipelines WHERE app_name = ?`, name)
	if err != nil {
		return nil, fmt.Errorf("store: list pipelines of %q: %w", name, err)
	}
	for _, id := range ids {
		if _, err := db.ExecContext(ctx, `DELETE FROM pipeline_webhook_triggers WHERE pipeline_id = ?`, id); err != nil {
			return nil, fmt.Errorf("store: delete pipeline webhook trigger of %q: %w", name, err)
		}
		namespaces = append(namespaces, PipelineWebhookSecretsKey(id))
	}

	patterns, err := db.stringsQuery(ctx, `SELECT DISTINCT branch_pattern FROM service_branch_env_overrides WHERE service_name = ? AND is_secret = 1`, name)
	if err != nil {
		return nil, fmt.Errorf("store: list secret branch overrides of %q: %w", name, err)
	}
	for _, p := range patterns {
		namespaces = append(namespaces, BranchEnvOverrideSecretsKey(name, p))
	}
	if len(patterns) > 0 {
		if _, err := db.ExecContext(ctx, `DELETE FROM service_branch_env_overrides WHERE service_name = ? AND is_secret = 1`, name); err != nil {
			return nil, fmt.Errorf("store: delete secret branch overrides of %q: %w", name, err)
		}
	}
	return namespaces, nil
}

func (db *DB) stringsQuery(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
