package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Target types a binding can point at.
const (
	BindingTargetApp   = "app"
	BindingTargetStack = "stack"
	BindingTargetOrg   = "org"
)

// Errors returned by the connection binding store.
var (
	ErrConnectionBindingExists   = errors.New("store: connection binding already exists")
	ErrConnectionBindingNotFound = errors.New("store: connection binding not found")
)

// ConnectionBinding links a connection ref to a target for one role. Rules is JSON validated by the caller.
type ConnectionBinding struct {
	ID         string
	Ref        string
	TargetType string
	TargetID   string
	Role       string
	Rules      string
	CreatedBy  string
	CreatedAt  time.Time
}

// NewConnectionBindingID returns a fresh binding id.
func NewConnectionBindingID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("store: generate connection binding id: %w", err)
	}
	return "cb_" + hex.EncodeToString(b), nil
}

const connectionBindingCols = "id, ref, target_type, target_id, role, rules, created_by, created_at"

func scanConnectionBinding(scan func(dest ...any) error) (ConnectionBinding, error) {
	var b ConnectionBinding
	var created string
	if err := scan(&b.ID, &b.Ref, &b.TargetType, &b.TargetID, &b.Role, &b.Rules, &b.CreatedBy, &created); err != nil {
		return ConnectionBinding{}, err
	}
	b.CreatedAt = parseOptionalTime(created)
	return b, nil
}

// CreateConnectionBinding inserts b, minting the id and timestamp when empty.
func (db *DB) CreateConnectionBinding(ctx context.Context, b ConnectionBinding) (ConnectionBinding, error) {
	if b.ID == "" {
		id, err := NewConnectionBindingID()
		if err != nil {
			return ConnectionBinding{}, err
		}
		b.ID = id
	}
	if b.Rules == "" {
		b.Rules = "{}"
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}
	_, err := db.ExecContext(ctx, `INSERT INTO connection_bindings (`+connectionBindingCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		b.ID, b.Ref, b.TargetType, b.TargetID, b.Role, b.Rules, b.CreatedBy, b.CreatedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return ConnectionBinding{}, ErrConnectionBindingExists
		}
		return ConnectionBinding{}, fmt.Errorf("store: create connection binding: %w", err)
	}
	return b, nil
}

func (db *DB) queryConnectionBindings(ctx context.Context, where string, args ...any) ([]ConnectionBinding, error) {
	rows, err := db.QueryContext(ctx, "SELECT "+connectionBindingCols+" FROM connection_bindings "+where+" ORDER BY created_at, id", args...)
	if err != nil {
		return nil, fmt.Errorf("store: list connection bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []ConnectionBinding{}
	for rows.Next() {
		b, err := scanConnectionBinding(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan connection binding: %w", err)
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ListConnectionBindingsByTarget returns the bindings of one app, stack or org.
func (db *DB) ListConnectionBindingsByTarget(ctx context.Context, targetType, targetID string) ([]ConnectionBinding, error) {
	return db.queryConnectionBindings(ctx, "WHERE target_type = ? AND target_id = ?", targetType, targetID)
}

// ListConnectionBindingsByRef returns every binding of one connection.
func (db *DB) ListConnectionBindingsByRef(ctx context.Context, ref string) ([]ConnectionBinding, error) {
	return db.queryConnectionBindings(ctx, "WHERE ref = ?", ref)
}

// CountConnectionBindingsByRef returns binding counts keyed by ref.
func (db *DB) CountConnectionBindingsByRef(ctx context.Context) (map[string]int, error) {
	rows, err := db.QueryContext(ctx, `SELECT ref, COUNT(*) FROM connection_bindings GROUP BY ref`)
	if err != nil {
		return nil, fmt.Errorf("store: count connection bindings: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]int{}
	for rows.Next() {
		var ref string
		var n int
		if err := rows.Scan(&ref, &n); err != nil {
			return nil, fmt.Errorf("store: scan connection binding count: %w", err)
		}
		out[ref] = n
	}
	return out, rows.Err()
}

// DeleteConnectionBinding removes one binding or returns ErrConnectionBindingNotFound.
func (db *DB) DeleteConnectionBinding(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM connection_bindings WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete connection binding: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrConnectionBindingNotFound
	}
	return nil
}

// Ref prefixes of the stored connection kinds, shared with the hub read model.
const (
	ConnectionRefGit           = "git:"
	ConnectionRefRegistry      = "registry:"
	ConnectionRefSecret        = "secret:"
	ConnectionRefCloud         = "cloud:"
	ConnectionRefEventWebhook  = "webhook:event:"
	ConnectionRefStatusWebhook = "webhook:status:"
)

type connExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// forgetConnectionRef drops a removed connection's bindings and sidecar, so a
// later connection reusing the ref starts unbound at the instance default.
func forgetConnectionRef(ctx context.Context, ex connExecer, ref string) error {
	if _, err := ex.ExecContext(ctx, `DELETE FROM connection_bindings WHERE ref = ?`, ref); err != nil {
		return fmt.Errorf("store: clear bindings of %q: %w", ref, err)
	}
	if _, err := ex.ExecContext(ctx, `DELETE FROM connection_meta WHERE ref = ?`, ref); err != nil {
		return fmt.Errorf("store: clear connection meta of %q: %w", ref, err)
	}
	return nil
}

// forgetBindingTarget drops a deleted app's or org's bindings and detaches the
// connections scoped to it: the scope id is cleared, which covers no target and
// leaves the scope to an instance admin, so a reused name inherits nothing.
func forgetBindingTarget(ctx context.Context, ex connExecer, targetType, id string) error {
	if _, err := ex.ExecContext(ctx, `DELETE FROM connection_bindings WHERE target_type = ? AND target_id = ?`, targetType, id); err != nil {
		return fmt.Errorf("store: clear bindings of %s %q: %w", targetType, id, err)
	}
	if _, err := ex.ExecContext(ctx, `UPDATE connection_meta SET scope_id = '', updated_at = ? WHERE scope_type = ? AND scope_id = ?`,
		time.Now().UTC().Format(time.RFC3339Nano), targetType, id); err != nil {
		return fmt.Errorf("store: detach connections scoped to %s %q: %w", targetType, id, err)
	}
	return nil
}
