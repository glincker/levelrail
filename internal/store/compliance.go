package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Access review and evidence pack states.
const (
	AccessReviewOpen      = "open"
	AccessReviewCompleted = "completed"

	AccessDecisionKeep   = "keep"
	AccessDecisionRevoke = "revoke"

	AccessSubjectUser       = "user"
	AccessSubjectTeamMember = "team_member"
	AccessSubjectAPIToken   = "api_token"
	AccessSubjectEnvGrant   = "env_grant"

	EvidencePackRunning  = "running"
	EvidencePackComplete = "complete"
	EvidencePackFailed   = "failed"
)

// ErrComplianceNotFound is returned for an unknown campaign, item or pack.
var ErrComplianceNotFound = errors.New("store: compliance record not found")

// AccessReviewCampaign is one review round. Times are RFC3339 strings.
type AccessReviewCampaign struct {
	ID          string
	Name        string
	Status      string
	DueAt       string
	RepeatDays  int
	DueDays     int
	CreatedBy   string
	CreatedAt   string
	CompletedAt string
}

// AccessReviewItem is one subject a reviewer keeps or revokes.
type AccessReviewItem struct {
	ID           string
	CampaignID   string
	Kind         string
	SubjectID    string
	Label        string
	Role         string
	Detail       string
	LastActiveAt string
	Decision     string
	DecidedBy    string
	DecidedAt    string
	AppliedAt    string
	ApplyError   string
}

// EvidencePack is one generated (or generating) evidence archive.
type EvidencePack struct {
	ID         string
	RangeFrom  string
	RangeTo    string
	Status     string
	Step       string
	Progress   int
	FileName   string
	SizeBytes  int64
	SHA256     string
	Error      string
	CreatedBy  string
	CreatedAt  string
	FinishedAt string
}

const campaignCols = `id, name, status, due_at, repeat_days, due_days, created_by, created_at, completed_at`

func scanCampaign(scan func(...any) error) (AccessReviewCampaign, error) {
	var c AccessReviewCampaign
	err := scan(&c.ID, &c.Name, &c.Status, &c.DueAt, &c.RepeatDays, &c.DueDays, &c.CreatedBy, &c.CreatedAt, &c.CompletedAt)
	return c, err
}

// CreateAccessReviewCampaign inserts a campaign and its items atomically.
func (db *DB) CreateAccessReviewCampaign(ctx context.Context, c AccessReviewCampaign, items []AccessReviewItem) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: create access review: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO access_review_campaigns (`+campaignCols+`) VALUES (?,?,?,?,?,?,?,?,?)`,
		c.ID, c.Name, c.Status, c.DueAt, c.RepeatDays, c.DueDays, c.CreatedBy, c.CreatedAt, c.CompletedAt); err != nil {
		return fmt.Errorf("store: create access review: %w", err)
	}
	for _, it := range items {
		if _, err := tx.ExecContext(ctx, `INSERT INTO access_review_items
			(id, campaign_id, kind, subject_id, label, role, detail, last_active_at) VALUES (?,?,?,?,?,?,?,?)`,
			it.ID, c.ID, it.Kind, it.SubjectID, it.Label, it.Role, it.Detail, it.LastActiveAt); err != nil {
			return fmt.Errorf("store: create access review item: %w", err)
		}
	}
	return tx.Commit()
}

// GetAccessReviewCampaign returns one campaign.
func (db *DB) GetAccessReviewCampaign(ctx context.Context, id string) (AccessReviewCampaign, error) {
	c, err := scanCampaign(db.QueryRowContext(ctx, `SELECT `+campaignCols+` FROM access_review_campaigns WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrComplianceNotFound
	}
	if err != nil {
		return c, fmt.Errorf("store: get access review: %w", err)
	}
	return c, nil
}

// ListAccessReviewCampaigns returns campaigns newest first.
func (db *DB) ListAccessReviewCampaigns(ctx context.Context) ([]AccessReviewCampaign, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+campaignCols+` FROM access_review_campaigns ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("store: list access reviews: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AccessReviewCampaign
	for rows.Next() {
		c, err := scanCampaign(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan access review: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

const itemCols = `id, campaign_id, kind, subject_id, label, role, detail, last_active_at, decision, decided_by, decided_at, applied_at, apply_error`

func scanItem(scan func(...any) error) (AccessReviewItem, error) {
	var i AccessReviewItem
	err := scan(&i.ID, &i.CampaignID, &i.Kind, &i.SubjectID, &i.Label, &i.Role, &i.Detail, &i.LastActiveAt,
		&i.Decision, &i.DecidedBy, &i.DecidedAt, &i.AppliedAt, &i.ApplyError)
	return i, err
}

// ListAccessReviewItems returns a campaign's rows in a stable order.
func (db *DB) ListAccessReviewItems(ctx context.Context, campaignID string) ([]AccessReviewItem, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+itemCols+` FROM access_review_items WHERE campaign_id = ? ORDER BY kind, label, id`, campaignID)
	if err != nil {
		return nil, fmt.Errorf("store: list access review items: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []AccessReviewItem
	for rows.Next() {
		i, err := scanItem(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan access review item: %w", err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// DecideAccessReviewItem records a keep or revoke decision, clearing any
// earlier apply error so a reviewer can retry or change their mind.
func (db *DB) DecideAccessReviewItem(ctx context.Context, campaignID, itemID, decision, by, at string) error {
	res, err := db.ExecContext(ctx, `UPDATE access_review_items SET decision = ?, decided_by = ?, decided_at = ?, apply_error = ''
		WHERE campaign_id = ? AND id = ? AND applied_at = ''`, decision, by, at, campaignID, itemID)
	if err != nil {
		return fmt.Errorf("store: decide access review item: %w", err)
	}
	return rowsAffectedOrNotFound(res, ErrComplianceNotFound, "decide access review item %q", itemID)
}

// SetAccessReviewItemApplied stamps a revocation as applied, or records why it failed.
func (db *DB) SetAccessReviewItemApplied(ctx context.Context, itemID, appliedAt, applyErr string) error {
	_, err := db.ExecContext(ctx, `UPDATE access_review_items SET applied_at = ?, apply_error = ? WHERE id = ?`, appliedAt, applyErr, itemID)
	if err != nil {
		return fmt.Errorf("store: mark access review item: %w", err)
	}
	return nil
}

// CompleteAccessReviewCampaign closes an open campaign.
func (db *DB) CompleteAccessReviewCampaign(ctx context.Context, id, at string) error {
	res, err := db.ExecContext(ctx, `UPDATE access_review_campaigns SET status = ?, completed_at = ? WHERE id = ? AND status = ?`,
		AccessReviewCompleted, at, id, AccessReviewOpen)
	if err != nil {
		return fmt.Errorf("store: complete access review: %w", err)
	}
	return rowsAffectedOrNotFound(res, ErrComplianceNotFound, "complete access review %q", id)
}

const packCols = `id, range_from, range_to, status, step, progress, file_name, size_bytes, sha256, error, created_by, created_at, finished_at`

func scanPack(scan func(...any) error) (EvidencePack, error) {
	var p EvidencePack
	err := scan(&p.ID, &p.RangeFrom, &p.RangeTo, &p.Status, &p.Step, &p.Progress, &p.FileName, &p.SizeBytes, &p.SHA256, &p.Error, &p.CreatedBy, &p.CreatedAt, &p.FinishedAt)
	return p, err
}

// CreateEvidencePack inserts a pack row, normally in the running state.
func (db *DB) CreateEvidencePack(ctx context.Context, p EvidencePack) error {
	_, err := db.ExecContext(ctx, `INSERT INTO evidence_packs (`+packCols+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.RangeFrom, p.RangeTo, p.Status, p.Step, p.Progress, p.FileName, p.SizeBytes, p.SHA256, p.Error, p.CreatedBy, p.CreatedAt, p.FinishedAt)
	if err != nil {
		return fmt.Errorf("store: create evidence pack: %w", err)
	}
	return nil
}

// SetEvidencePackProgress updates the step and percent of a running pack.
func (db *DB) SetEvidencePackProgress(ctx context.Context, id, step string, progress int) error {
	_, err := db.ExecContext(ctx, `UPDATE evidence_packs SET step = ?, progress = ? WHERE id = ? AND status = ?`, step, progress, id, EvidencePackRunning)
	if err != nil {
		return fmt.Errorf("store: evidence pack progress: %w", err)
	}
	return nil
}

// FinishEvidencePack moves a running pack to complete or failed.
func (db *DB) FinishEvidencePack(ctx context.Context, p EvidencePack) error {
	_, err := db.ExecContext(ctx, `UPDATE evidence_packs SET status = ?, step = ?, progress = ?, file_name = ?, size_bytes = ?, sha256 = ?, error = ?, finished_at = ?
		WHERE id = ? AND status = ?`, p.Status, p.Step, p.Progress, p.FileName, p.SizeBytes, p.SHA256, p.Error, p.FinishedAt, p.ID, EvidencePackRunning)
	if err != nil {
		return fmt.Errorf("store: finish evidence pack: %w", err)
	}
	return nil
}

// FailRunningEvidencePacks marks packs a restart orphaned as failed.
func (db *DB) FailRunningEvidencePacks(ctx context.Context, at, reason string) error {
	_, err := db.ExecContext(ctx, `UPDATE evidence_packs SET status = ?, error = ?, finished_at = ? WHERE status = ?`,
		EvidencePackFailed, reason, at, EvidencePackRunning)
	if err != nil {
		return fmt.Errorf("store: fail running evidence packs: %w", err)
	}
	return nil
}

// GetEvidencePack returns one pack.
func (db *DB) GetEvidencePack(ctx context.Context, id string) (EvidencePack, error) {
	p, err := scanPack(db.QueryRowContext(ctx, `SELECT `+packCols+` FROM evidence_packs WHERE id = ?`, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrComplianceNotFound
	}
	if err != nil {
		return p, fmt.Errorf("store: get evidence pack: %w", err)
	}
	return p, nil
}

// ListEvidencePacks returns packs newest first.
func (db *DB) ListEvidencePacks(ctx context.Context) ([]EvidencePack, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+packCols+` FROM evidence_packs ORDER BY created_at DESC LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("store: list evidence packs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []EvidencePack
	for rows.Next() {
		p, err := scanPack(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan evidence pack: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
