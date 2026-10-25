package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrCIStackTrustNotFound is returned when no trust matches the id and stack.
var ErrCIStackTrustNotFound = errors.New("store: ci stack trust not found")

// ErrCIStackTrustExists is returned when the same claim set is already trusted for the stack.
var ErrCIStackTrustExists = errors.New("store: ci stack trust already exists")

// ErrInvalidCIStackTrust rejects a trust without a stack, repository, ref pattern or valid verbs.
var ErrInvalidCIStackTrust = errors.New("store: invalid ci stack trust")

// CIStackTrust lets one CI identity (repository, ref pattern, optional environment and subject
// pattern) exchange its OIDC token for a short-lived token holding Verbs on one stack.
type CIStackTrust struct {
	ID             string
	StackID        string
	Repository     string
	RefPattern     string
	Environment    string
	SubjectPattern string
	Verbs          []string
	CreatedBy      string
	CreatedAt      time.Time
}

// NewCIStackTrustID mints a random trust id.
func NewCIStackTrustID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate ci stack trust id: %w", err)
	}
	return "cst_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

const ciStackTrustColumns = `id, stack_id, repository, ref_pattern, environment, subject_pattern, verbs, created_by, created_at`

func scanCIStackTrust(scan func(dest ...any) error) (CIStackTrust, error) {
	var t CIStackTrust
	var verbs, created string
	if err := scan(&t.ID, &t.StackID, &t.Repository, &t.RefPattern, &t.Environment, &t.SubjectPattern, &verbs, &t.CreatedBy, &created); err != nil {
		return CIStackTrust{}, err
	}
	t.Verbs = strings.Split(verbs, ",")
	// A row whose verbs no longer parse is an error, so the exchange fails closed.
	if !ValidStackVerbs(t.Verbs) {
		return CIStackTrust{}, ErrInvalidCIStackTrust
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return t, nil
}

// SaveCIStackTrust inserts a trust.
func (db *DB) SaveCIStackTrust(ctx context.Context, t CIStackTrust) error {
	if t.ID == "" || t.StackID == "" || t.Repository == "" || t.RefPattern == "" || !ValidStackVerbs(t.Verbs) {
		return ErrInvalidCIStackTrust
	}
	_, err := db.ExecContext(ctx, `
		INSERT INTO ci_stack_trusts (`+ciStackTrustColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.StackID, t.Repository, t.RefPattern, t.Environment, t.SubjectPattern, strings.Join(t.Verbs, ","), t.CreatedBy, t.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrCIStackTrustExists
		}
		return fmt.Errorf("store: save ci stack trust: %w", err)
	}
	return nil
}

func (db *DB) queryCIStackTrusts(ctx context.Context, where string, args ...any) ([]CIStackTrust, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+ciStackTrustColumns+` FROM ci_stack_trusts WHERE `+where+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list ci stack trusts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CIStackTrust
	for rows.Next() {
		t, err := scanCIStackTrust(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan ci stack trust: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListCIStackTrusts returns the trusts of one stack.
func (db *DB) ListCIStackTrusts(ctx context.Context, stackID string) ([]CIStackTrust, error) {
	return db.queryCIStackTrusts(ctx, "stack_id = ?", stackID)
}

// ListCIStackTrustsForRepo returns the trusts on one repository and stack, the candidates an incoming token is matched against.
func (db *DB) ListCIStackTrustsForRepo(ctx context.Context, repository, stackID string) ([]CIStackTrust, error) {
	return db.queryCIStackTrusts(ctx, "repository = ? AND stack_id = ?", repository, stackID)
}

// DeleteCIStackTrust removes a trust of the stack and returns it for the audit trail.
func (db *DB) DeleteCIStackTrust(ctx context.Context, stackID, id string) (CIStackTrust, error) {
	t, err := scanCIStackTrust(db.QueryRowContext(ctx, `SELECT `+ciStackTrustColumns+` FROM ci_stack_trusts WHERE id = ? AND stack_id = ?`, id, stackID).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return CIStackTrust{}, ErrCIStackTrustNotFound
	}
	if err != nil {
		return CIStackTrust{}, fmt.Errorf("store: get ci stack trust: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM ci_stack_trusts WHERE id = ? AND stack_id = ?`, id, stackID); err != nil {
		return CIStackTrust{}, fmt.Errorf("store: delete ci stack trust: %w", err)
	}
	return t, nil
}
