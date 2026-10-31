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

// ErrCIOIDCTrustNotFound is returned when no trust matches the id and app.
var ErrCIOIDCTrustNotFound = errors.New("store: ci oidc trust not found")

// ErrCIOIDCTrustExists is returned when the same provider, issuer, repository, ref and environment is already trusted for the app.
var ErrCIOIDCTrustExists = errors.New("store: ci oidc trust already exists")

// CIOIDCTrust lets one repository ref (and optional environment) deploy one app with a short-lived token.
type CIOIDCTrust struct {
	ID          string
	ServiceName string
	Repository  string
	RefPattern  string
	Environment string
	PolicyID    string
	CreatedBy   string
	CreatedAt   time.Time
	// Provider is github, gitlab or circleci; Issuer is empty for github, whose issuer is fixed.
	Provider         string
	Issuer           string
	ProtectedRefOnly bool
}

// NewCIOIDCTrustID mints a random trust id.
func NewCIOIDCTrustID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("store: generate ci oidc trust id: %w", err)
	}
	return "cit_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

const ciOIDCTrustColumns = `id, service_name, repository, ref_pattern, environment, policy_id, created_by, created_at, provider, issuer, protected_ref_only`

func scanCIOIDCTrust(scan func(dest ...any) error) (CIOIDCTrust, error) {
	var t CIOIDCTrust
	var created string
	if err := scan(&t.ID, &t.ServiceName, &t.Repository, &t.RefPattern, &t.Environment, &t.PolicyID, &t.CreatedBy, &created, &t.Provider, &t.Issuer, &t.ProtectedRefOnly); err != nil {
		return CIOIDCTrust{}, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return t, nil
}

// SaveCIOIDCTrust inserts a trust.
func (db *DB) SaveCIOIDCTrust(ctx context.Context, t CIOIDCTrust) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO ci_oidc_trusts (`+ciOIDCTrustColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, t.ServiceName, t.Repository, t.RefPattern, t.Environment, t.PolicyID, t.CreatedBy, t.CreatedAt.UTC().Format(time.RFC3339Nano), ciTrustProvider(t.Provider), t.Issuer, t.ProtectedRefOnly)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrCIOIDCTrustExists
		}
		return fmt.Errorf("store: save ci oidc trust: %w", err)
	}
	return nil
}

func (db *DB) queryCIOIDCTrusts(ctx context.Context, where string, args ...any) ([]CIOIDCTrust, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+ciOIDCTrustColumns+` FROM ci_oidc_trusts WHERE `+where+` ORDER BY created_at`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list ci oidc trusts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []CIOIDCTrust
	for rows.Next() {
		t, err := scanCIOIDCTrust(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan ci oidc trust: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListCIOIDCTrusts returns the trusts of one app.
func (db *DB) ListCIOIDCTrusts(ctx context.Context, serviceName string) ([]CIOIDCTrust, error) {
	return db.queryCIOIDCTrusts(ctx, "service_name = ?", serviceName)
}

// ListCIOIDCTrustsForRepo returns the trusts on one repository and app, the candidates an incoming token is matched against.
func (db *DB) ListCIOIDCTrustsForRepo(ctx context.Context, repository, serviceName string) ([]CIOIDCTrust, error) {
	return db.queryCIOIDCTrusts(ctx, "repository = ? AND service_name = ?", repository, serviceName)
}

// DeleteCIOIDCTrust removes a trust and returns it so the caller can drop its policy.
func (db *DB) DeleteCIOIDCTrust(ctx context.Context, serviceName, id string) (CIOIDCTrust, error) {
	t, err := scanCIOIDCTrust(db.QueryRowContext(ctx, `SELECT `+ciOIDCTrustColumns+` FROM ci_oidc_trusts WHERE id = ? AND service_name = ?`, id, serviceName).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return CIOIDCTrust{}, ErrCIOIDCTrustNotFound
	}
	if err != nil {
		return CIOIDCTrust{}, fmt.Errorf("store: get ci oidc trust: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM ci_oidc_trusts WHERE id = ?`, id); err != nil {
		return CIOIDCTrust{}, fmt.Errorf("store: delete ci oidc trust: %w", err)
	}
	return t, nil
}

func ciTrustProvider(p string) string {
	if p == "" {
		return "github"
	}
	return p
}

// DeleteCIOIDCTrustsForApp removes every trust of an app and the IAM policies minted for them, so a recreated app of the same name starts without CI access.
func (db *DB) DeleteCIOIDCTrustsForApp(ctx context.Context, serviceName string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: delete ci oidc trusts for app: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM iam_policies WHERE id IN (SELECT policy_id FROM ci_oidc_trusts WHERE service_name = ? AND policy_id != '')`, serviceName); err != nil {
		return fmt.Errorf("store: delete ci oidc trust policies for app: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM ci_oidc_trusts WHERE service_name = ?`, serviceName); err != nil {
		return fmt.Errorf("store: delete ci oidc trusts for app: %w", err)
	}
	return tx.Commit()
}
