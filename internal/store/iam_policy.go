package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

// Policy is one IAM-style policy document (iam.go's Document type holds
// the actual Allow/Deny statements): additive on top of a principal's
// flat Abilities list, either narrowing a broad ability down to specific
// resources (a Deny statement) or granting an ability scoped to
// specific resources without granting it globally (an Allow statement).
type Policy struct {
	ID          string
	Name        string
	Description string
	Document    string // raw JSON; internal/api's iam package parses/validates it
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// PolicyAttachment links one Policy to one principal (a user or an API
// token). PrincipalType is "user" or "token", matching audit_log's own
// actor_type discriminator convention rather than a real foreign key,
// since a principal isn't a single referenceable table.
type PolicyAttachment struct {
	ID            string
	PolicyID      string
	PrincipalType string
	PrincipalID   string
	CreatedAt     time.Time
}

// PrincipalTypeUser and PrincipalTypeToken are the only two valid
// PolicyAttachment.PrincipalType values.
const (
	PrincipalTypeUser  = "user"
	PrincipalTypeToken = "token"
)

var (
	// ErrPolicyNotFound is returned by GetPolicy/DeletePolicy/UpdatePolicy
	// when no row matches.
	ErrPolicyNotFound = errors.New("store: iam policy not found")
	// ErrPolicyNameExists is SavePolicy's failure mode when name is
	// already taken by a different row.
	ErrPolicyNameExists = errors.New("store: iam policy name already exists")
)

// NewPolicyID mints a random policy ID, the same fixed-length
// crypto/rand-plus-base64 scheme NewDeployAttemptID already establishes.
func NewPolicyID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate policy id: %w", err)
	}
	return "pol_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// NewPolicyAttachmentID mints a random attachment ID, same scheme as
// NewPolicyID.
func NewPolicyAttachmentID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate policy attachment id: %w", err)
	}
	return "pola_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

// SavePolicy creates a new policy. Policies are never updated in place
// except via UpdatePolicy (name/description/document); unlike
// SaveAPIToken this genuinely is meant to change over the policy's
// life, since a policy document is the whole point of editing one,
// unlike a token's fixed-at-mint-time abilities. On failure it
// re-checks by name to classify the cause, the same pattern
// CreateUser uses for its own unique constraint.
func (db *DB) SavePolicy(ctx context.Context, p Policy) error {
	now := formatTime(time.Now())
	_, err := db.ExecContext(ctx, `
		INSERT INTO iam_policies (id, name, description, document, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, p.ID, p.Name, p.Description, p.Document, now, now)
	if err == nil {
		return nil
	}
	if existing, getErr := db.getPolicyByName(ctx, p.Name); getErr == nil && existing.ID != p.ID {
		return ErrPolicyNameExists
	}
	return fmt.Errorf("store: save policy %q: %w", p.ID, err)
}

// UpdatePolicy replaces an existing policy's name/description/document,
// bumping updated_at. Returns ErrPolicyNotFound if id doesn't exist,
// ErrPolicyNameExists if name collides with a different row.
func (db *DB) UpdatePolicy(ctx context.Context, id, name, description, document string) error {
	now := formatTime(time.Now())
	res, err := db.ExecContext(ctx, `
		UPDATE iam_policies SET name = ?, description = ?, document = ?, updated_at = ?
		WHERE id = ?
	`, name, description, document, now, id)
	if err != nil {
		if existing, getErr := db.getPolicyByName(ctx, name); getErr == nil && existing.ID != id {
			return ErrPolicyNameExists
		}
		return fmt.Errorf("store: update policy %q: %w", id, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update policy %q: %w", id, err)
	}
	if n == 0 {
		return ErrPolicyNotFound
	}
	return nil
}

func (db *DB) getPolicyByName(ctx context.Context, name string) (*Policy, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, description, document, created_at, updated_at
		FROM iam_policies WHERE name = ?
	`, name)
	return scanPolicy(row.Scan)
}

// GetPolicy returns the policy with this ID, or ErrPolicyNotFound.
func (db *DB) GetPolicy(ctx context.Context, id string) (*Policy, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, description, document, created_at, updated_at
		FROM iam_policies WHERE id = ?
	`, id)
	p, err := scanPolicy(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPolicyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("store: get policy %q: %w", id, err)
	}
	return p, nil
}

// ListPolicies returns every policy, ordered by name.
func (db *DB) ListPolicies(ctx context.Context) ([]Policy, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, description, document, created_at, updated_at
		FROM iam_policies ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list policies: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Policy
	for rows.Next() {
		p, err := scanPolicy(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan policy row: %w", err)
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate policy rows: %w", err)
	}
	return out, nil
}

// DeletePolicy removes a policy and (via ON DELETE CASCADE) every
// attachment referencing it. Idempotent: deleting a missing ID is not
// an error, matching DeleteDesiredService's own convention.
func (db *DB) DeletePolicy(ctx context.Context, id string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM iam_policies WHERE id = ?`, id); err != nil {
		return fmt.Errorf("store: delete policy %q: %w", id, err)
	}
	return nil
}

// AttachPolicy links policyID to one principal. Idempotent: attaching
// an already-attached (policy, principal) pair is a no-op, not an
// error, since the caller's intent ("this principal should have this
// policy") is already satisfied.
func (db *DB) AttachPolicy(ctx context.Context, id, policyID, principalType, principalID string) error {
	now := formatTime(time.Now())
	_, err := db.ExecContext(ctx, `
		INSERT INTO iam_policy_attachments (id, policy_id, principal_type, principal_id, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (policy_id, principal_type, principal_id) DO NOTHING
	`, id, policyID, principalType, principalID, now)
	if err != nil {
		return fmt.Errorf("store: attach policy %q to %s %q: %w", policyID, principalType, principalID, err)
	}
	return nil
}

// DetachPolicy removes the link between policyID and one principal.
// Idempotent, matching DeletePolicy's own convention.
func (db *DB) DetachPolicy(ctx context.Context, policyID, principalType, principalID string) error {
	_, err := db.ExecContext(ctx, `
		DELETE FROM iam_policy_attachments
		WHERE policy_id = ? AND principal_type = ? AND principal_id = ?
	`, policyID, principalType, principalID)
	if err != nil {
		return fmt.Errorf("store: detach policy %q from %s %q: %w", policyID, principalType, principalID, err)
	}
	return nil
}

// ListPoliciesForPrincipal returns every policy attached to one
// principal (a user or a token), ordered by name: exactly the set
// internal/api's authorization check evaluates for a given caller.
func (db *DB) ListPoliciesForPrincipal(ctx context.Context, principalType, principalID string) ([]Policy, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT p.id, p.name, p.description, p.document, p.created_at, p.updated_at
		FROM iam_policies p
		JOIN iam_policy_attachments a ON a.policy_id = p.id
		WHERE a.principal_type = ? AND a.principal_id = ?
		ORDER BY p.name
	`, principalType, principalID)
	if err != nil {
		return nil, fmt.Errorf("store: list policies for %s %q: %w", principalType, principalID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []Policy
	for rows.Next() {
		p, err := scanPolicy(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan policy row: %w", err)
		}
		out = append(out, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate policy rows: %w", err)
	}
	return out, nil
}

// ListAttachmentsForPolicy returns every principal a policy is attached
// to, ordered by creation time: the management UI/CLI's "who does this
// policy apply to" view.
func (db *DB) ListAttachmentsForPolicy(ctx context.Context, policyID string) ([]PolicyAttachment, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, policy_id, principal_type, principal_id, created_at
		FROM iam_policy_attachments WHERE policy_id = ? ORDER BY created_at
	`, policyID)
	if err != nil {
		return nil, fmt.Errorf("store: list attachments for policy %q: %w", policyID, err)
	}
	defer func() { _ = rows.Close() }()

	var out []PolicyAttachment
	for rows.Next() {
		var a PolicyAttachment
		var createdAt string
		if err := rows.Scan(&a.ID, &a.PolicyID, &a.PrincipalType, &a.PrincipalID, &createdAt); err != nil {
			return nil, fmt.Errorf("store: scan policy attachment row: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse policy attachment created_at: %w", err)
		}
		a.CreatedAt = t
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate policy attachment rows: %w", err)
	}
	return out, nil
}

func scanPolicy(scan func(dest ...any) error) (*Policy, error) {
	var p Policy
	var createdAt, updatedAt string
	if err := scan(&p.ID, &p.Name, &p.Description, &p.Document, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	ct, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	ut, err := time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return nil, fmt.Errorf("parse updated_at: %w", err)
	}
	p.CreatedAt, p.UpdatedAt = ct, ut
	return &p, nil
}
