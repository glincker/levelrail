package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// The lifecycle a node_provisions row moves through. Recomputed live
// against the provider API and the nodes table on every read
// (internal/api/node_provision.go), not advanced by a background worker.
const (
	NodeProvisionStatusCreating   = "creating"
	NodeProvisionStatusBooting    = "booting"
	NodeProvisionStatusInstalling = "installing"
	NodeProvisionStatusEnrolling  = "enrolling"
	NodeProvisionStatusReady      = "ready"
	NodeProvisionStatusFailed     = "failed"
)

// NodeProvision tracks one cloud VM created on an operator's behalf,
// through to the join-token enrollment it's expected to complete.
// Correlated to the real store.Node it produces by Name: the cloud-init
// script sets APP_NODE_NAME to this row's own Name, so the enrolled
// node's Name is how a later read finds it (nodes.name is already
// UNIQUE, migrations/0008_nodes.sql).
type NodeProvision struct {
	ID               string
	Provider         string
	Region           string
	Size             string
	Name             string
	Role             string
	Status           string
	ProviderServerID string
	IPAddress        string
	// NodeID is set once the enrolled node is found by Name.
	NodeID        string
	FailureReason string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ErrNodeProvisionNotFound is returned by GetNodeProvision when id
// doesn't match any row.
var ErrNodeProvisionNotFound = errors.New("store: node provision not found")

// NodeProviderSecretsKey is the internal/secrets serviceName a cloud
// provider's API token is stored under (envKey NodeProviderTokenEnvKey),
// mirroring RegistryCredentialSecretsKey's "<kind>/<id>" shape, one
// namespace per provider rather than per provision: the credential is
// platform-wide, entered once in Settings, reused by every provision
// call against that provider.
func NodeProviderSecretsKey(provider string) string {
	return "node-provider/" + provider
}

// NodeProviderTokenEnvKey is the fixed envKey within that namespace.
const NodeProviderTokenEnvKey = "token"

// SaveNodeProvision inserts a new node provision row. IDs are minted by
// the caller before this call, the same "generate before the INSERT"
// pattern BackupTarget/RegistryCredential use.
func (db *DB) SaveNodeProvision(ctx context.Context, p NodeProvision) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_provisions (id, provider, region, size, name, role, status, provider_server_id, ip_address, node_id, failure_reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.Provider, p.Region, p.Size, p.Name, p.Role, p.Status, p.ProviderServerID, p.IPAddress, p.NodeID, p.FailureReason, formatTime(p.CreatedAt), formatTime(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save node provision %q: %w", p.ID, err)
	}
	return nil
}

// GetNodeProvision returns the node provision with this ID, or
// ErrNodeProvisionNotFound.
func (db *DB) GetNodeProvision(ctx context.Context, id string) (NodeProvision, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, provider, region, size, name, role, status, provider_server_id, ip_address, node_id, failure_reason, created_at, updated_at
		FROM node_provisions WHERE id = ?
	`, id)
	p, err := scanNodeProvision(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return NodeProvision{}, ErrNodeProvisionNotFound
	}
	if err != nil {
		return NodeProvision{}, fmt.Errorf("store: get node provision %q: %w", id, err)
	}
	return p, nil
}

// ListNodeProvisions returns every node provision, newest first.
func (db *DB) ListNodeProvisions(ctx context.Context) ([]NodeProvision, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, provider, region, size, name, role, status, provider_server_id, ip_address, node_id, failure_reason, created_at, updated_at
		FROM node_provisions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list node provisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []NodeProvision
	for rows.Next() {
		p, err := scanNodeProvision(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan node provision row: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate node provision rows: %w", err)
	}
	return out, nil
}

// UpdateNodeProvisionStatus replaces status, providerServerID, ipAddress,
// nodeID and failureReason for an existing row: the fields internal/api's
// live status recomputation (and CreateServer's own initial result) can
// learn between one write and the next. An empty providerServerID leaves
// the stored value unchanged, since most callers are only updating
// status/ip/node/failure after the server already has one. Returns
// ErrNodeProvisionNotFound if id doesn't exist.
func (db *DB) UpdateNodeProvisionStatus(ctx context.Context, id, status, providerServerID, ipAddress, nodeID, failureReason string, updatedAt time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE node_provisions
		SET status = ?,
		    provider_server_id = CASE WHEN ? != '' THEN ? ELSE provider_server_id END,
		    ip_address = ?, node_id = ?, failure_reason = ?, updated_at = ?
		WHERE id = ?
	`, status, providerServerID, providerServerID, ipAddress, nodeID, failureReason, formatTime(updatedAt), id)
	if err != nil {
		return fmt.Errorf("store: update node provision %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrNodeProvisionNotFound, "update node provision %q", id)
}

func scanNodeProvision(scan func(dest ...any) error) (NodeProvision, error) {
	var (
		p                    NodeProvision
		createdAt, updatedAt string
	)
	if err := scan(&p.ID, &p.Provider, &p.Region, &p.Size, &p.Name, &p.Role, &p.Status,
		&p.ProviderServerID, &p.IPAddress, &p.NodeID, &p.FailureReason, &createdAt, &updatedAt); err != nil {
		return NodeProvision{}, err
	}
	var err error
	if p.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return NodeProvision{}, fmt.Errorf("parse created_at: %w", err)
	}
	if p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return NodeProvision{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return p, nil
}
