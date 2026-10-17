package plugin

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Lifecycle states of an installed plugin.
const (
	StateInstalled = "installed"
	StateEnabled   = "enabled"
	StateDisabled  = "disabled"
	StateRemoving  = "removing"
)

// Sentinel errors. Callers match with errors.Is.
var (
	ErrNotFound    = errors.New("plugin: not found")
	ErrExists      = errors.New("plugin: already installed")
	ErrNotApproved = errors.New("plugin: abilities have not been approved")
	ErrState       = errors.New("plugin: not allowed in its current state")
)

// InvalidError marks a request the operator can fix, as opposed to an internal failure.
type InvalidError struct{ Msg string }

func (e *InvalidError) Error() string { return "plugin: " + e.Msg }

func invalid(format string, a ...any) error { return &InvalidError{Msg: fmt.Sprintf(format, a...)} }

// Record is one installed plugin as stored.
type Record struct {
	Name               string   `json:"name"`
	Version            string   `json:"version"`
	Kind               string   `json:"kind"`
	Publisher          string   `json:"publisher"`
	Description        string   `json:"description"`
	Image              string   `json:"image"`
	Digest             string   `json:"digest"`
	Manifest           string   `json:"manifest"`
	Signature          string   `json:"-"`
	KeyID              string   `json:"key_id"`
	RequestedAbilities []string `json:"requested_abilities"`
	ApprovedAbilities  []string `json:"approved_abilities"`
	OutboundHosts      []string `json:"outbound_hosts"`
	ConfigJSON         string   `json:"config_json"`
	State              string   `json:"state"`
	ApprovedAt         string   `json:"approved_at"`
	ApprovedBy         string   `json:"approved_by"`
	InstalledAt        string   `json:"installed_at"`
	InstalledBy        string   `json:"installed_by"`
	UpdatedAt          string   `json:"updated_at"`
	LastError          string   `json:"last_error"`
}

// Store persists plugins and the trust list.
type Store interface {
	ListPlugins(ctx context.Context) ([]Record, error)
	GetPlugin(ctx context.Context, name string) (Record, error)
	InsertPlugin(ctx context.Context, r Record) error
	UpdatePlugin(ctx context.Context, r Record) error
	SetLastError(ctx context.Context, name, msg, updatedAt string) error
	DeletePlugin(ctx context.Context, name string) error
	ListKeys(ctx context.Context) ([]TrustedKey, error)
	AddKey(ctx context.Context, k TrustedKey) error
	RemoveKey(ctx context.Context, keyID string) error
}

// DBTX is the slice of *sql.DB (and the control plane's store.DB) the SQL store needs.
type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// SQLStore implements Store over the control plane database.
type SQLStore struct{ db DBTX }

// NewSQLStore wraps db.
func NewSQLStore(db DBTX) *SQLStore { return &SQLStore{db: db} }

const recordColumns = `name, version, kind, publisher, description, image, digest, manifest, signature, key_id,
	requested_abilities, approved_abilities, outbound_hosts, config_json, state,
	approved_at, approved_by, installed_at, installed_by, updated_at, last_error`

func marshalList(l []string) string {
	if l == nil {
		l = []string{}
	}
	b, err := json.Marshal(l)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func unmarshalList(s string) []string {
	out := []string{}
	if err := json.Unmarshal([]byte(s), &out); err != nil || out == nil {
		return []string{}
	}
	return out
}

type scanner interface{ Scan(dest ...any) error }

func scanRecord(sc scanner) (Record, error) {
	var r Record
	var req, appr, hosts string
	err := sc.Scan(&r.Name, &r.Version, &r.Kind, &r.Publisher, &r.Description, &r.Image, &r.Digest, &r.Manifest, &r.Signature, &r.KeyID,
		&req, &appr, &hosts, &r.ConfigJSON, &r.State, &r.ApprovedAt, &r.ApprovedBy, &r.InstalledAt, &r.InstalledBy, &r.UpdatedAt, &r.LastError)
	if err != nil {
		return Record{}, err
	}
	r.RequestedAbilities, r.ApprovedAbilities, r.OutboundHosts = unmarshalList(req), unmarshalList(appr), unmarshalList(hosts)
	return r, nil
}

// ListPlugins returns every plugin, by name.
func (s *SQLStore) ListPlugins(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+recordColumns+` FROM plugins ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("plugin: list: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []Record{}
	for rows.Next() {
		r, err := scanRecord(rows)
		if err != nil {
			return nil, fmt.Errorf("plugin: scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetPlugin returns one plugin or ErrNotFound.
func (s *SQLStore) GetPlugin(ctx context.Context, name string) (Record, error) {
	r, err := scanRecord(s.db.QueryRowContext(ctx, `SELECT `+recordColumns+` FROM plugins WHERE name = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("plugin: get %q: %w", name, err)
	}
	return r, nil
}

// InsertPlugin adds a plugin, or returns ErrExists.
func (s *SQLStore) InsertPlugin(ctx context.Context, r Record) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO plugins (`+recordColumns+`) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.Name, r.Version, r.Kind, r.Publisher, r.Description, r.Image, r.Digest, r.Manifest, r.Signature, r.KeyID,
		marshalList(r.RequestedAbilities), marshalList(r.ApprovedAbilities), marshalList(r.OutboundHosts), r.ConfigJSON, r.State,
		r.ApprovedAt, r.ApprovedBy, r.InstalledAt, r.InstalledBy, r.UpdatedAt, r.LastError)
	if err != nil {
		if existing, gerr := s.GetPlugin(ctx, r.Name); gerr == nil && existing.Name == r.Name {
			return ErrExists
		}
		return fmt.Errorf("plugin: insert %q: %w", r.Name, err)
	}
	return nil
}

// UpdatePlugin writes the mutable columns of a plugin.
func (s *SQLStore) UpdatePlugin(ctx context.Context, r Record) error {
	res, err := s.db.ExecContext(ctx, `UPDATE plugins SET state = ?, approved_abilities = ?, approved_at = ?, approved_by = ?, updated_at = ?, last_error = ? WHERE name = ?`,
		r.State, marshalList(r.ApprovedAbilities), r.ApprovedAt, r.ApprovedBy, r.UpdatedAt, r.LastError, r.Name)
	if err != nil {
		return fmt.Errorf("plugin: update %q: %w", r.Name, err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetLastError records the latest runtime failure without touching state or approvals.
func (s *SQLStore) SetLastError(ctx context.Context, name, msg, updatedAt string) error {
	if _, err := s.db.ExecContext(ctx, `UPDATE plugins SET last_error = ?, updated_at = ? WHERE name = ?`, msg, updatedAt, name); err != nil {
		return fmt.Errorf("plugin: set last error %q: %w", name, err)
	}
	return nil
}

// DeletePlugin removes a plugin row.
func (s *SQLStore) DeletePlugin(ctx context.Context, name string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM plugins WHERE name = ?`, name); err != nil {
		return fmt.Errorf("plugin: delete %q: %w", name, err)
	}
	return nil
}

// ListKeys returns the trust list.
func (s *SQLStore) ListKeys(ctx context.Context) ([]TrustedKey, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key_id, name, public_key, added_at, added_by FROM plugin_trusted_keys ORDER BY added_at, key_id`)
	if err != nil {
		return nil, fmt.Errorf("plugin: list keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []TrustedKey{}
	for rows.Next() {
		var k TrustedKey
		if err := rows.Scan(&k.KeyID, &k.Name, &k.PublicKey, &k.AddedAt, &k.AddedBy); err != nil {
			return nil, fmt.Errorf("plugin: scan key: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// AddKey trusts a publisher key.
func (s *SQLStore) AddKey(ctx context.Context, k TrustedKey) error {
	if _, err := s.db.ExecContext(ctx, `INSERT INTO plugin_trusted_keys (key_id, name, public_key, added_at, added_by) VALUES (?,?,?,?,?)
		ON CONFLICT (key_id) DO UPDATE SET name = excluded.name`, k.KeyID, k.Name, k.PublicKey, k.AddedAt, k.AddedBy); err != nil {
		return fmt.Errorf("plugin: add key: %w", err)
	}
	return nil
}

// RemoveKey stops trusting a key.
func (s *SQLStore) RemoveKey(ctx context.Context, keyID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM plugin_trusted_keys WHERE key_id = ?`, keyID)
	if err != nil {
		return fmt.Errorf("plugin: remove key: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
