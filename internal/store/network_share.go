package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNetworkShareNotFound is returned by GetNetworkShare, GetNetworkShareByName,
// UpdateNetworkShare, and DeleteNetworkShare when id/name doesn't match any row.
var ErrNetworkShareNotFound = errors.New("store: network share not found")

// Network share protocols, matching internal/docker's own NFS/CIFS
// driver-opts translation.
const (
	NetworkShareProtocolNFS  = "nfs"
	NetworkShareProtocolCIFS = "cifs"
)

// NetworkShare is an external NFS or CIFS/SMB export this control plane
// can mount as a Docker local-driver volume. No password field: a CIFS
// share's credential goes through internal/secrets, keyed by
// NetworkShareSecretsKey(id), the same split RegistryCredential uses for
// its own password. Username is CIFS-only; empty for an NFS share, which
// authenticates by source IP/export rules instead.
type NetworkShare struct {
	ID           string
	Name         string
	Protocol     string
	Host         string
	RemotePath   string
	MountOptions string
	Username     string
	CreatedAt    string
}

// NetworkShareSecretsKey is the internal/secrets serviceName a network
// share's CIFS password is stored under (envKey "password").
func NetworkShareSecretsKey(id string) string {
	return "network-share/" + id
}

// SaveNetworkShare inserts a new network share row. IDs are minted by
// the caller before this call, the same "generate before the INSERT"
// pattern RegistryCredential uses.
func (db *DB) SaveNetworkShare(ctx context.Context, s NetworkShare) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO network_shares (id, name, protocol, host, remote_path, mount_options, username, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, s.ID, s.Name, s.Protocol, s.Host, s.RemotePath, s.MountOptions, s.Username, s.CreatedAt)
	if err != nil {
		return fmt.Errorf("store: save network share %q: %w", s.ID, err)
	}
	return nil
}

func scanNetworkShare(row interface {
	Scan(dest ...any) error
}) (NetworkShare, error) {
	var s NetworkShare
	err := row.Scan(&s.ID, &s.Name, &s.Protocol, &s.Host, &s.RemotePath, &s.MountOptions, &s.Username, &s.CreatedAt)
	return s, err
}

// GetNetworkShare returns the network share with this ID, or
// ErrNetworkShareNotFound.
func (db *DB) GetNetworkShare(ctx context.Context, id string) (NetworkShare, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, protocol, host, remote_path, mount_options, username, created_at
		FROM network_shares
		WHERE id = ?
	`, id)
	s, err := scanNetworkShare(row)
	if errors.Is(err, sql.ErrNoRows) {
		return NetworkShare{}, ErrNetworkShareNotFound
	}
	if err != nil {
		return NetworkShare{}, fmt.Errorf("store: get network share %q: %w", id, err)
	}
	return s, nil
}

// GetNetworkShareByName returns the network share with this Name, or
// ErrNetworkShareNotFound. A volume's network share reference resolves
// by name, not opaque ID, the same reasoning
// GetRegistryCredentialByName's own doc comment gives for app.yaml.
func (db *DB) GetNetworkShareByName(ctx context.Context, name string) (NetworkShare, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, protocol, host, remote_path, mount_options, username, created_at
		FROM network_shares
		WHERE name = ?
	`, name)
	s, err := scanNetworkShare(row)
	if errors.Is(err, sql.ErrNoRows) {
		return NetworkShare{}, ErrNetworkShareNotFound
	}
	if err != nil {
		return NetworkShare{}, fmt.Errorf("store: get network share by name %q: %w", name, err)
	}
	return s, nil
}

// ListNetworkShares returns every network share, oldest first.
func (db *DB) ListNetworkShares(ctx context.Context) ([]NetworkShare, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, protocol, host, remote_path, mount_options, username, created_at
		FROM network_shares
		ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list network shares: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []NetworkShare
	for rows.Next() {
		s, err := scanNetworkShare(rows)
		if err != nil {
			return nil, fmt.Errorf("store: scan network share row: %w", err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate network share rows: %w", err)
	}
	return out, nil
}

// UpdateNetworkShare replaces name/protocol/host/remote_path/mount_options/username
// for an existing row, the same full-replace contract UpdateRegistryCredential
// uses for its own sibling resource. Rotating a CIFS password is a
// separate write to internal/secrets under this row's existing
// NetworkShareSecretsKey, not this function's concern. Returns
// ErrNetworkShareNotFound if id doesn't exist.
func (db *DB) UpdateNetworkShare(ctx context.Context, id, name, protocol, host, remotePath, mountOptions, username string) error {
	res, err := db.ExecContext(ctx, `
		UPDATE network_shares
		SET name = ?, protocol = ?, host = ?, remote_path = ?, mount_options = ?, username = ?
		WHERE id = ?
	`, name, protocol, host, remotePath, mountOptions, username, id)
	if err != nil {
		return fmt.Errorf("store: update network share %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrNetworkShareNotFound, "update network share %q", id)
}

// DeleteNetworkShare removes a network share row. It does not touch
// internal/secrets: the caller deletes the password secret separately,
// after this succeeds, the same ordering DeleteRegistryCredential uses.
// Returns ErrNetworkShareNotFound if id doesn't exist.
func (db *DB) DeleteNetworkShare(ctx context.Context, id string) error {
	res, err := db.ExecContext(ctx, `DELETE FROM network_shares WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("store: delete network share %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrNetworkShareNotFound, "delete network share %q", id)
}
