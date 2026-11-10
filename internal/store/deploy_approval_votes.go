package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DeployApprovalVote is one distinct principal's approval of one pending
// deploy approval. ApproverKey is the human behind the principal when there
// is one, so two tokens of the same user share a key. Binding fingerprints
// the exact request the vote was cast on.
type DeployApprovalVote struct {
	ApprovalID   string
	ApproverKey  string
	ApproverType string
	ApproverID   string
	ApproverName string
	Binding      string
	// ImageDigest is the registry digest a tag resolved to when the vote was
	// cast, empty for an image already pinned by digest.
	ImageDigest string
	CreatedAt   string
}

// RecordDeployApprovalVote inserts v once; inserted is false when this
// approver already voted on the approval, so a retry never counts twice.
func (db *DB) RecordDeployApprovalVote(ctx context.Context, v DeployApprovalVote) (bool, error) {
	res, err := db.ExecContext(ctx, `
		INSERT INTO deploy_approval_votes (approval_id, approver_key, approver_type, approver_id, approver_name, binding, image_digest, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (approval_id, approver_key) DO NOTHING
	`, v.ApprovalID, v.ApproverKey, v.ApproverType, v.ApproverID, v.ApproverName, v.Binding, v.ImageDigest, v.CreatedAt)
	return deployApprovalAffected(res, err, "record deploy approval vote", v.ApprovalID)
}

// ReplaceDeployApprovalVote swaps the same person's stored vote for v, only
// while the row still equals prev, so two racing replacements cannot both win.
func (db *DB) ReplaceDeployApprovalVote(ctx context.Context, v, prev DeployApprovalVote) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_approval_votes
		SET approver_type = ?, approver_id = ?, approver_name = ?, binding = ?, image_digest = ?, created_at = ?
		WHERE approval_id = ? AND approver_key = ? AND approver_type = ? AND approver_id = ? AND binding = ? AND image_digest = ?
	`, v.ApproverType, v.ApproverID, v.ApproverName, v.Binding, v.ImageDigest, v.CreatedAt,
		v.ApprovalID, v.ApproverKey, prev.ApproverType, prev.ApproverID, prev.Binding, prev.ImageDigest)
	return deployApprovalAffected(res, err, "replace deploy approval vote", v.ApprovalID)
}

// ListDeployApprovalVotes returns an approval's votes, oldest first.
func (db *DB) ListDeployApprovalVotes(ctx context.Context, approvalID string) ([]DeployApprovalVote, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT approval_id, approver_key, approver_type, approver_id, approver_name, binding, image_digest, created_at
		FROM deploy_approval_votes WHERE approval_id = ? ORDER BY created_at, approver_key
	`, approvalID)
	if err != nil {
		return nil, fmt.Errorf("store: list deploy approval votes %q: %w", approvalID, err)
	}
	defer func() {
		_ = rows.Close()
	}()
	var out []DeployApprovalVote
	for rows.Next() {
		var v DeployApprovalVote
		if err := rows.Scan(&v.ApprovalID, &v.ApproverKey, &v.ApproverType, &v.ApproverID, &v.ApproverName, &v.Binding, &v.ImageDigest, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("store: scan deploy approval vote: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list deploy approval votes %q: %w", approvalID, err)
	}
	return out, nil
}

// EnvironmentRequiredApprovers returns how many distinct approvers envID
// needs; 1 when no policy row exists.
func (db *DB) EnvironmentRequiredApprovers(ctx context.Context, envID string) (int, error) {
	var n int
	err := db.QueryRowContext(ctx, `SELECT required_approvers FROM environment_approval_policies WHERE environment_id = ?`, envID).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return 1, nil
	}
	if err != nil {
		return 0, fmt.Errorf("store: environment required approvers %q: %w", envID, err)
	}
	return n, nil
}

// SetEnvironmentRequiredApprovers upserts envID's approver count.
func (db *DB) SetEnvironmentRequiredApprovers(ctx context.Context, envID string, n int, updatedAt string) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO environment_approval_policies (environment_id, required_approvers, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (environment_id) DO UPDATE SET required_approvers = excluded.required_approvers, updated_at = excluded.updated_at
	`, envID, n, updatedAt)
	if err != nil {
		return fmt.Errorf("store: set environment required approvers %q: %w", envID, err)
	}
	return nil
}
