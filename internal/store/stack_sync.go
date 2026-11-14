package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// StackSyncConfig is the git source a stack plans from. CredentialRef is a
// name such as "app:web", never a secret value.
type StackSyncConfig struct {
	RepoURL         string
	Branch          string
	Path            string
	CredentialRef   string
	IntervalSeconds int
	WebhookOnly     bool
	Enabled         bool
	Apply           bool
	Prune           bool
	Deploy          bool
}

// StackSyncStatus is the latest sync result: commit, plan hash and counts, and
// a stable code on failure. NotifiedSHA is the last commit a pending event named.
type StackSyncStatus struct {
	LastSHA     string
	NotifiedSHA string
	PlanHash    string
	Counts      map[string]int
	Pending     bool
	Code        string
	CheckedAt   time.Time
	// AppliedSHA is the last commit handed to the apply path; Revision is what it made.
	AppliedSHA string
	Revision   int
	// Detail is a file:line for a failure, never repository content.
	Detail string
}

// StackSyncRecord is one stack's sync row joined with the stack name.
type StackSyncRecord struct {
	StackID   string
	StackName string
	Config    StackSyncConfig
	Status    StackSyncStatus
	UpdatedAt time.Time
}

const stackSyncCols = `y.stack_id, s.name, y.repo_url, y.branch, y.path, y.credential_ref, y.interval_seconds, y.webhook_only, y.enabled,
	y.last_sha, y.notified_sha, y.plan_hash, y.counts_json, y.pending, y.code, y.checked_at, y.updated_at,
	y.apply, y.prune, y.deploy, y.applied_sha, y.revision, y.detail`

func scanStackSync(sc interface{ Scan(...any) error }) (StackSyncRecord, error) {
	var r StackSyncRecord
	var webhookOnly, enabled, pending, apply, prune, deploy int
	var counts, checked, updated string
	if err := sc.Scan(&r.StackID, &r.StackName, &r.Config.RepoURL, &r.Config.Branch, &r.Config.Path, &r.Config.CredentialRef,
		&r.Config.IntervalSeconds, &webhookOnly, &enabled, &r.Status.LastSHA, &r.Status.NotifiedSHA, &r.Status.PlanHash,
		&counts, &pending, &r.Status.Code, &checked, &updated,
		&apply, &prune, &deploy, &r.Status.AppliedSHA, &r.Status.Revision, &r.Status.Detail); err != nil {
		return StackSyncRecord{}, err
	}
	r.Config.WebhookOnly, r.Config.Enabled, r.Status.Pending = webhookOnly != 0, enabled != 0, pending != 0
	r.Config.Apply, r.Config.Prune, r.Config.Deploy = apply != 0, prune != 0, deploy != 0
	_ = json.Unmarshal([]byte(counts), &r.Status.Counts)
	r.Status.CheckedAt, r.UpdatedAt = planJobTime(checked), planJobTime(updated)
	return r, nil
}

// StackSyncSecretsKey is the sealed secrets namespace of a stack's sync webhook,
// keyed by id so a rename keeps the secret.
func StackSyncSecretsKey(stackID string) string {
	return "stack-sync/" + stackID
}

// GetStackSync returns the sync row of a stack; found is false when it has none.
func (db *DB) GetStackSync(ctx context.Context, stackID string) (StackSyncRecord, bool, error) {
	r, err := scanStackSync(db.QueryRowContext(ctx, "SELECT "+stackSyncCols+" FROM stack_sync y JOIN stacks s ON s.id = y.stack_id WHERE y.stack_id = ?", stackID))
	if errors.Is(err, sql.ErrNoRows) {
		return StackSyncRecord{}, false, nil
	}
	if err != nil {
		return StackSyncRecord{}, false, fmt.Errorf("store: get stack sync: %w", err)
	}
	return r, true, nil
}

// ListEnabledStackSyncs returns every stack with sync turned on.
func (db *DB) ListEnabledStackSyncs(ctx context.Context) ([]StackSyncRecord, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+stackSyncCols+" FROM stack_sync y JOIN stacks s ON s.id = y.stack_id WHERE y.enabled = 1 ORDER BY s.name")
	if err != nil {
		return nil, fmt.Errorf("store: list stack syncs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []StackSyncRecord
	for rows.Next() {
		r, err := scanStackSync(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan stack sync: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PutStackSyncConfig stores the source of a stack. A changed source forgets the
// notified commit so the next plan of the new source is announced.
func (db *DB) PutStackSyncConfig(ctx context.Context, stackID string, c StackSyncConfig, now time.Time) error {
	_, err := db.ExecContext(ctx, `INSERT INTO stack_sync (stack_id, repo_url, branch, path, credential_ref, interval_seconds, webhook_only, enabled, apply, prune, deploy, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (stack_id) DO UPDATE SET
			notified_sha = CASE WHEN stack_sync.repo_url = excluded.repo_url AND stack_sync.branch = excluded.branch AND stack_sync.path = excluded.path THEN stack_sync.notified_sha ELSE '' END,
			repo_url = excluded.repo_url, branch = excluded.branch, path = excluded.path, credential_ref = excluded.credential_ref,
			interval_seconds = excluded.interval_seconds, webhook_only = excluded.webhook_only, enabled = excluded.enabled,
			apply = excluded.apply, prune = excluded.prune, deploy = excluded.deploy, updated_at = excluded.updated_at`,
		stackID, c.RepoURL, c.Branch, c.Path, c.CredentialRef, c.IntervalSeconds, boolInt(c.WebhookOnly), boolInt(c.Enabled),
		boolInt(c.Apply), boolInt(c.Prune), boolInt(c.Deploy), fmtLockTime(now))
	if err != nil {
		return fmt.Errorf("store: put stack sync: %w", err)
	}
	return nil
}

// SetStackSyncStatus records the latest sync result of a stack.
func (db *DB) SetStackSyncStatus(ctx context.Context, stackID string, st StackSyncStatus) error {
	counts, err := json.Marshal(st.Counts)
	if err != nil || st.Counts == nil {
		counts = []byte("{}")
	}
	res, err := db.ExecContext(ctx, `UPDATE stack_sync SET last_sha = ?, notified_sha = ?, plan_hash = ?, counts_json = ?, pending = ?, code = ?, checked_at = ?,
		applied_sha = ?, revision = ?, detail = ? WHERE stack_id = ?`,
		st.LastSHA, st.NotifiedSHA, st.PlanHash, string(counts), boolInt(st.Pending), st.Code, fmtLockTime(st.CheckedAt),
		st.AppliedSHA, st.Revision, st.Detail, stackID)
	if err != nil {
		return fmt.Errorf("store: set stack sync status: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrStackNotFound
	}
	return nil
}
