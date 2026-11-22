package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Account lifecycle values; the column is free text so a provider may add more.
const (
	CloudAccountStatusActive   = "active"
	CloudAccountStatusDisabled = "disabled"
	CloudAccountStatusError    = "error"
)

// CloudAccountDefaultSuffix names the account the migration creates per provider.
const CloudAccountDefaultSuffix = "-default"

// ErrCloudAccountNotFound is returned when an account id matches no row.
var ErrCloudAccountNotFound = errors.New("store: cloud account not found")

// CloudAccount is one stored credential for one provider. Provider specific
// settings (regions allowlist, role ARN, external id, tier) live in Config so a
// new provider needs no schema change.
type CloudAccount struct {
	ID          string
	Provider    string
	DisplayName string
	AuthMethod  string
	// Config is a JSON object, never credentials.
	Config json.RawMessage
	// CredentialRef is the internal/secrets service name holding the secret.
	CredentialRef  string
	TeamID         string
	Status         string
	CreatedBy      string
	LastVerifiedAt *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// DefaultCloudAccountID is the id of a provider's migrated default account.
func DefaultCloudAccountID(provider string) string {
	return provider + CloudAccountDefaultSuffix
}

const cloudAccountColumns = `id, provider, display_name, auth_method, config, credential_ref, team_id, status, created_by, last_verified_at, created_at, updated_at`

// CreateCloudAccount inserts an account; the caller mints the id.
func (db *DB) CreateCloudAccount(ctx context.Context, a CloudAccount) error {
	if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.Provider) == "" {
		return errors.New("store: cloud account needs an id and a provider")
	}
	cfg, err := normalizeCloudConfig(a.Config)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if a.CreatedAt.IsZero() {
		a.CreatedAt = now
	}
	if a.UpdatedAt.IsZero() {
		a.UpdatedAt = a.CreatedAt
	}
	if a.Status == "" {
		a.Status = CloudAccountStatusActive
	}
	if a.AuthMethod == "" {
		a.AuthMethod = "api_key"
	}
	_, err = db.ExecContext(ctx, `INSERT INTO cloud_accounts (`+cloudAccountColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Provider, a.DisplayName, a.AuthMethod, cfg, a.CredentialRef, a.TeamID, a.Status, a.CreatedBy,
		formatTimeOrEmpty(derefTime(a.LastVerifiedAt)), formatTime(a.CreatedAt), formatTime(a.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: create cloud account %q: %w", a.ID, err)
	}
	return nil
}

// GetCloudAccount returns one account or ErrCloudAccountNotFound.
func (db *DB) GetCloudAccount(ctx context.Context, id string) (CloudAccount, error) {
	row := db.QueryRowContext(ctx, `SELECT `+cloudAccountColumns+` FROM cloud_accounts WHERE id = ?`, id)
	a, err := scanCloudAccount(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return CloudAccount{}, ErrCloudAccountNotFound
	}
	if err != nil {
		return CloudAccount{}, fmt.Errorf("store: get cloud account %q: %w", id, err)
	}
	return a, nil
}

// ListCloudAccounts returns every account, optionally for one provider, oldest first.
func (db *DB) ListCloudAccounts(ctx context.Context, provider string) ([]CloudAccount, error) {
	q := `SELECT ` + cloudAccountColumns + ` FROM cloud_accounts`
	var args []any
	if provider != "" {
		q += ` WHERE provider = ?`
		args = append(args, provider)
	}
	rows, err := db.QueryContext(ctx, q+` ORDER BY created_at, id`, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list cloud accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []CloudAccount{}
	for rows.Next() {
		a, err := scanCloudAccount(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan cloud account: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate cloud accounts: %w", err)
	}
	return out, nil
}

// UpdateCloudAccount replaces the editable fields (name, method, config,
// credential ref, team, status). Provider and creator never change.
func (db *DB) UpdateCloudAccount(ctx context.Context, a CloudAccount) error {
	cfg, err := normalizeCloudConfig(a.Config)
	if err != nil {
		return err
	}
	res, err := db.ExecContext(ctx, `UPDATE cloud_accounts SET display_name = ?, auth_method = ?, config = ?, credential_ref = ?,
		team_id = ?, status = ?, updated_at = ? WHERE id = ?`,
		a.DisplayName, a.AuthMethod, cfg, a.CredentialRef, a.TeamID, a.Status, formatTime(time.Now().UTC()), a.ID)
	if err != nil {
		return fmt.Errorf("store: update cloud account %q: %w", a.ID, err)
	}
	return rowsAffectedOrNotFound(res, ErrCloudAccountNotFound, "update cloud account %q", a.ID)
}

// MarkCloudAccountVerified records a successful credential check.
func (db *DB) MarkCloudAccountVerified(ctx context.Context, id string, at time.Time) error {
	res, err := db.ExecContext(ctx, `UPDATE cloud_accounts SET last_verified_at = ?, status = ?, updated_at = ? WHERE id = ?`,
		formatTime(at), CloudAccountStatusActive, formatTime(at), id)
	if err != nil {
		return fmt.Errorf("store: verify cloud account %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrCloudAccountNotFound, "verify cloud account %q", id)
}

// DeleteCloudAccount removes the account; its grants cascade, provisions keep their account_id.
func (db *DB) DeleteCloudAccount(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM cloud_accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete cloud account %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrCloudAccountNotFound, "delete cloud account %q", id)
}

func normalizeCloudConfig(c json.RawMessage) (string, error) {
	if len(strings.TrimSpace(string(c))) == 0 {
		return "{}", nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(c, &obj); err != nil {
		return "", fmt.Errorf("store: cloud account config must be a JSON object: %w", err)
	}
	return string(c), nil
}

func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func scanCloudAccount(scan func(dest ...any) error) (CloudAccount, error) {
	var (
		a                               CloudAccount
		cfg, verified, created, updated string
	)
	if err := scan(&a.ID, &a.Provider, &a.DisplayName, &a.AuthMethod, &cfg, &a.CredentialRef, &a.TeamID,
		&a.Status, &a.CreatedBy, &verified, &created, &updated); err != nil {
		return CloudAccount{}, err
	}
	a.Config = json.RawMessage(cfg)
	var err error
	if verified != "" {
		t, perr := time.Parse(time.RFC3339Nano, verified)
		if perr != nil {
			return CloudAccount{}, fmt.Errorf("parse last_verified_at: %w", perr)
		}
		a.LastVerifiedAt = &t
	}
	if a.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return CloudAccount{}, fmt.Errorf("parse created_at: %w", err)
	}
	if a.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return CloudAccount{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return a, nil
}
