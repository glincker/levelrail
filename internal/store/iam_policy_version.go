package store

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"time"
)

// PolicyVersion is one saved document of a policy.
type PolicyVersion struct {
	ID          string
	PolicyID    string
	Version     int
	Name        string
	Description string
	Document    string
	Actor       string
	CreatedAt   time.Time
}

// SavePolicyVersion records the policy's current document as the next version.
func (db *DB) SavePolicyVersion(ctx context.Context, policyID, name, description, document, actor string) error {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("store: generate policy version id: %w", err)
	}
	id := "polv_" + base64.RawURLEncoding.EncodeToString(buf)
	_, err := db.ExecContext(ctx, `
		INSERT INTO iam_policy_versions (id, policy_id, version, name, description, document, actor, created_at)
		SELECT ?, ?, COALESCE(MAX(version), 0) + 1, ?, ?, ?, ?, ? FROM iam_policy_versions WHERE policy_id = ?
	`, id, policyID, name, description, document, actor, formatTime(time.Now()), policyID)
	if err != nil {
		return fmt.Errorf("store: save policy version for %q: %w", policyID, err)
	}
	return nil
}

// ListPolicyVersions returns a policy's versions, newest first.
func (db *DB) ListPolicyVersions(ctx context.Context, policyID string) ([]PolicyVersion, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, policy_id, version, name, description, document, actor, created_at
		FROM iam_policy_versions WHERE policy_id = ? ORDER BY version DESC
	`, policyID)
	if err != nil {
		return nil, fmt.Errorf("store: list policy versions for %q: %w", policyID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []PolicyVersion
	for rows.Next() {
		var v PolicyVersion
		var createdAt string
		if err := rows.Scan(&v.ID, &v.PolicyID, &v.Version, &v.Name, &v.Description, &v.Document, &v.Actor, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan policy version row: %w", err)
		}
		ct, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse policy version created_at: %w", err)
		}
		v.CreatedAt = ct
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate policy version rows: %w", err)
	}
	return out, nil
}
