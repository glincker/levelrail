package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

// ErrDeployApprovalNotFound is returned by GetDeployApproval when id
// doesn't match any row.
var ErrDeployApprovalNotFound = errors.New("store: deploy approval not found")

// Deploy approval actions: what kind of desired-state change is gated
// behind the approval, DeployApproval.Action's valid values.
const (
	DeployApprovalActionDeploy  = "deploy"
	DeployApprovalActionPromote = "promote"
	// DeployApprovalActionMoveApp and DeployApprovalActionMoveDatabase gate
	// retagging a resource into or out of a protected environment; Image
	// then carries the target environment id.
	DeployApprovalActionMoveApp      = "move_app_environment"
	DeployApprovalActionMoveDatabase = "move_database_environment"
)

// Deploy approval statuses, DeployApproval.Status's valid values.
const (
	DeployApprovalStatusPending  = "pending"
	DeployApprovalStatusApproved = "approved"
	DeployApprovalStatusRejected = "rejected"
	DeployApprovalStatusExpired  = "expired"
	// DeployApprovalStatusApproving is held by one approver while the change
	// applies, so a racing approve, reject or expiry cannot land on the row.
	DeployApprovalStatusApproving = "approving"
)

// DeployApproval is one pending-or-decided two-person approval gate on a
// deploy/promote into a protected environment (migrations/0116): the
// real workflow behind Environment.Protected, replacing the old
// same-actor confirm-flag gate (environments.go's
// environmentNeedsConfirmation, internal/api). RequestedBy/ApprovedBy
// use the same PrincipalTypeUser/PrincipalTypeToken vocabulary
// iam_policy.go already established, so "the same actor can't approve
// their own request" compares on the identical (type, id) pair
// requireAbilityDecided resolves for every other ability check.
type DeployApproval struct {
	ID string
	// ServiceName is the app whose desired image actually changes once
	// approved.
	ServiceName string
	// SourceServiceName is the app a promote action's image comes from;
	// empty for a plain deploy action.
	SourceServiceName string
	EnvironmentID     string
	// Action is one of the DeployApprovalAction* constants.
	Action string
	// Image is the target image tag ServiceName's desired state will be
	// pointed at once this is approved.
	Image string
	// Status is one of the DeployApprovalStatus* constants.
	Status string

	RequestedByType string
	RequestedBy     string
	RequestedByName string
	// RequesterUserID is the human behind the request, resolved when it was
	// made; empty for an ownerless token. RequesterRecorded is false on rows
	// from before the column existed.
	RequesterUserID   string
	RequesterRecorded bool

	// ApprovedByType/ApprovedBy/ApprovedByName are empty until Status
	// leaves pending; "ApprovedBy" is this row's decider whether the
	// outcome was approve or reject, matching the "who decided" framing
	// audit_log's own ActorID already uses regardless of whether the
	// decision was an allow or a deny.
	ApprovedByType string
	ApprovedBy     string
	ApprovedByName string
	// Reason is an optional operator-supplied note, only ever set on
	// reject.
	Reason string

	CreatedAt string
	ExpiresAt string
	// DecidedAt is empty while Status is pending.
	DecidedAt string

	// FreezeOverride is the requester's freeze override note, empty when
	// the request did not override a freeze.
	FreezeOverride string
	// Pull requires a fresh registry resolution when the deploy applies.
	Pull bool
	// IncludeEnv applies a promotion's added and removed env keys too.
	IncludeEnv bool
	// PromoteEnv is the JSON env diff snapshotted when the promotion was requested.
	PromoteEnv string
	// ReleaseID is the release this approval applies when bindings are
	// authoritative; empty for a tag based approval.
	ReleaseID string
}

// deployApprovalIDPrefix mirrors auditEntryIDPrefix's own "short,
// greppable tag on an otherwise-opaque random ID" convention.
const deployApprovalIDPrefix = "apr_"

// NewDeployApprovalID generates an opaque, URL-safe deploy approval
// identifier, minted the same way NewAuditEntryID mints its own.
func NewDeployApprovalID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate deploy approval id: %w", err)
	}
	return deployApprovalIDPrefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

// SaveDeployApproval inserts a new deploy_approvals row, insert-only:
// a decision is recorded with the dedicated Approve/RejectDeployApproval
// methods below, never by re-inserting.
func (db *DB) SaveDeployApproval(ctx context.Context, a DeployApproval) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO deploy_approvals (
			id, service_name, source_service_name, environment_id, action, image, status,
			requested_by_type, requested_by, requested_by_name,
			approved_by_type, approved_by, approved_by_name, reason,
			created_at, expires_at, decided_at, freeze_override, pull, include_env, promote_env,
			requester_user_id, release_id
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.ServiceName, a.SourceServiceName, a.EnvironmentID, a.Action, a.Image, a.Status,
		a.RequestedByType, a.RequestedBy, a.RequestedByName,
		a.ApprovedByType, a.ApprovedBy, a.ApprovedByName, a.Reason,
		a.CreatedAt, a.ExpiresAt, a.DecidedAt, a.FreezeOverride, a.Pull, a.IncludeEnv, a.PromoteEnv,
		sql.NullString{String: a.RequesterUserID, Valid: a.RequesterRecorded},
		sql.NullString{String: a.ReleaseID, Valid: a.ReleaseID != ""})
	if err != nil {
		return fmt.Errorf("store: save deploy approval %q: %w", a.ID, err)
	}
	return nil
}

// GetDeployApproval returns the deploy approval with this ID, or
// ErrDeployApprovalNotFound.
func (db *DB) GetDeployApproval(ctx context.Context, id string) (DeployApproval, error) {
	row := db.QueryRowContext(ctx, deployApprovalSelect+`WHERE id = ?`, id)
	a, err := scanDeployApproval(row)
	if errors.Is(err, sql.ErrNoRows) {
		return DeployApproval{}, ErrDeployApprovalNotFound
	}
	if err != nil {
		return DeployApproval{}, fmt.Errorf("store: get deploy approval %q: %w", id, err)
	}
	return a, nil
}

// ListDeployApprovals returns deploy approvals, newest first, optionally
// filtered to one status (DeployApprovalStatus*, empty means "any") and
// one service name (empty means "any app"): GET /api/v1/deploy-approvals'
// own query parameters (internal/api/deploy_approvals.go).
func (db *DB) ListDeployApprovals(ctx context.Context, status, serviceName string) ([]DeployApproval, error) {
	query := deployApprovalSelect + `WHERE (? = '' OR status = ?) AND (? = '' OR service_name = ?) ORDER BY created_at DESC`
	rows, err := db.QueryContext(ctx, query, status, status, serviceName, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: list deploy approvals: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []DeployApproval
	for rows.Next() {
		a, err := scanDeployApproval(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan deploy approval row: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list deploy approvals: %w", err)
	}
	return out, nil
}

// DecideDeployApproval moves a pending approval to a terminal status
// (approved, rejected, or expired), recording who decided it and when.
// Only ever succeeds from status = 'pending': a caller racing to decide
// an already-decided or already-expired row gets ok=false rather than
// silently overwriting an earlier decision, the same optimistic-
// concurrency shape a single UPDATE ... WHERE status = 'pending' gives
// for free without a separate SELECT-then-UPDATE round trip.
func (db *DB) DecideDeployApproval(ctx context.Context, id, newStatus, approvedByType, approvedBy, approvedByName, reason, decidedAt string) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_approvals
		SET status = ?, approved_by_type = ?, approved_by = ?, approved_by_name = ?, reason = ?, decided_at = ?
		WHERE id = ? AND status = ?
	`, newStatus, approvedByType, approvedBy, approvedByName, reason, decidedAt, id, DeployApprovalStatusPending)
	if err != nil {
		return false, fmt.Errorf("store: decide deploy approval %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: decide deploy approval %q: %w", id, err)
	}
	return n > 0, nil
}

// ClaimDeployApproval moves a pending, unexpired approval to approving for one
// decider; only the caller that gets ok=true may apply the gated change.
// claimedAt is stored in decided_at and identifies this claim later.
func (db *DB) ClaimDeployApproval(ctx context.Context, id, approvedByType, approvedBy, approvedByName, claimedAt string) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_approvals
		SET status = ?, approved_by_type = ?, approved_by = ?, approved_by_name = ?, decided_at = ?
		WHERE id = ? AND status = ? AND expires_at > ?
	`, DeployApprovalStatusApproving, approvedByType, approvedBy, approvedByName, claimedAt,
		id, DeployApprovalStatusPending, claimedAt)
	return deployApprovalAffected(res, err, "claim deploy approval", id)
}

// FinishDeployApproval marks this claim approved once its change applied.
func (db *DB) FinishDeployApproval(ctx context.Context, id, claimedAt, decidedAt string) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_approvals SET status = ?, decided_at = ?
		WHERE id = ? AND status = ? AND decided_at = ?
	`, DeployApprovalStatusApproved, decidedAt, id, DeployApprovalStatusApproving, claimedAt)
	return deployApprovalAffected(res, err, "finish deploy approval", id)
}

// ReleaseDeployApproval returns this claim to pending with no decider, after
// its apply failed or its claimer vanished, so someone can decide it again.
func (db *DB) ReleaseDeployApproval(ctx context.Context, id, claimedAt string) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_approvals
		SET status = ?, approved_by_type = '', approved_by = '', approved_by_name = '', decided_at = ''
		WHERE id = ? AND status = ? AND decided_at = ?
	`, DeployApprovalStatusPending, id, DeployApprovalStatusApproving, claimedAt)
	return deployApprovalAffected(res, err, "release deploy approval", id)
}

func deployApprovalAffected(res sql.Result, err error, op, id string) (bool, error) {
	if err != nil {
		return false, fmt.Errorf("store: %s %q: %w", op, id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("store: %s %q: %w", op, id, err)
	}
	return n > 0, nil
}

const deployApprovalSelect = `
	SELECT id, service_name, source_service_name, environment_id, action, image, status,
		requested_by_type, requested_by, requested_by_name,
		approved_by_type, approved_by, approved_by_name, reason,
		created_at, expires_at, decided_at, freeze_override, pull, include_env, promote_env,
		requester_user_id, release_id
	FROM deploy_approvals
`

// scanDeployApproval uses rowScanner (backup_verification.go), the
// shared *sql.Row/*sql.Rows interface this package already established
// for the same GetX/ListX split.
func scanDeployApproval(s rowScanner) (DeployApproval, error) {
	var a DeployApproval
	var requester, releaseID sql.NullString
	err := s.Scan(
		&a.ID, &a.ServiceName, &a.SourceServiceName, &a.EnvironmentID, &a.Action, &a.Image, &a.Status,
		&a.RequestedByType, &a.RequestedBy, &a.RequestedByName,
		&a.ApprovedByType, &a.ApprovedBy, &a.ApprovedByName, &a.Reason,
		&a.CreatedAt, &a.ExpiresAt, &a.DecidedAt, &a.FreezeOverride, &a.Pull, &a.IncludeEnv, &a.PromoteEnv,
		&requester, &releaseID,
	)
	a.ReleaseID = releaseID.String
	a.RequesterUserID, a.RequesterRecorded = requester.String, requester.Valid
	return a, err
}
