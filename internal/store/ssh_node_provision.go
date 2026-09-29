package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// The lifecycle an ssh_node_provisions row moves through. Unlike
// NodeProvisionStatus* (recomputed live against a provider API), Status,
// DetectedOS, DetectedArch and Log here are pushed by the provisioning
// goroutine itself as the SSH session progresses (internal/sshprovision),
// since this control plane has a live connection to the remote host
// rather than a provider's own status endpoint to poll.
const (
	SSHNodeProvisionStatusConnecting = "connecting"
	SSHNodeProvisionStatusDetecting  = "detecting"
	SSHNodeProvisionStatusInstalling = "installing"
	SSHNodeProvisionStatusEnrolling  = "enrolling"
	SSHNodeProvisionStatusReady      = "ready"
	SSHNodeProvisionStatusFailed     = "failed"
)

// SSHNodeProvision tracks one SSH-driven agent install on a machine the
// operator already has, through to the join-token enrollment it's
// expected to complete. Correlated to the real store.Node it produces by
// Name, the same convention NodeProvision uses.
type SSHNodeProvision struct {
	ID            string
	Name          string
	Role          string
	Status        string
	DetectedOS    string
	DetectedArch  string
	NodeID        string
	FailureReason string
	// Log is the accumulated, human-readable progress of the SSH install
	// (internal/sshprovision.Event.Message lines), replaced wholesale on
	// each update rather than appended in SQL: the provisioning goroutine
	// is the sole writer for a given row's lifetime, so there is no
	// concurrent-append race to guard against.
	Log       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ErrSSHNodeProvisionNotFound is returned by GetSSHNodeProvision when id
// doesn't match any row.
var ErrSSHNodeProvisionNotFound = errors.New("store: ssh node provision not found")

// SaveSSHNodeProvision inserts a new SSH node provision row. IDs are
// minted by the caller before this call, the same "generate before the
// INSERT" pattern NodeProvision uses.
func (db *DB) SaveSSHNodeProvision(ctx context.Context, p SSHNodeProvision) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO ssh_node_provisions (id, name, role, status, detected_os, detected_arch, node_id, failure_reason, log, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.Name, p.Role, p.Status, p.DetectedOS, p.DetectedArch, p.NodeID, p.FailureReason, p.Log, formatTime(p.CreatedAt), formatTime(p.UpdatedAt))
	if err != nil {
		return fmt.Errorf("store: save ssh node provision %q: %w", p.ID, err)
	}
	return nil
}

// GetSSHNodeProvision returns the SSH node provision with this ID, or
// ErrSSHNodeProvisionNotFound.
func (db *DB) GetSSHNodeProvision(ctx context.Context, id string) (SSHNodeProvision, error) {
	row := db.QueryRowContext(ctx, `
		SELECT id, name, role, status, detected_os, detected_arch, node_id, failure_reason, log, created_at, updated_at
		FROM ssh_node_provisions WHERE id = ?
	`, id)
	p, err := scanSSHNodeProvision(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return SSHNodeProvision{}, ErrSSHNodeProvisionNotFound
	}
	if err != nil {
		return SSHNodeProvision{}, fmt.Errorf("store: get ssh node provision %q: %w", id, err)
	}
	return p, nil
}

// ListSSHNodeProvisions returns every SSH node provision, newest first.
func (db *DB) ListSSHNodeProvisions(ctx context.Context) ([]SSHNodeProvision, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, role, status, detected_os, detected_arch, node_id, failure_reason, log, created_at, updated_at
		FROM ssh_node_provisions ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("store: list ssh node provisions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SSHNodeProvision
	for rows.Next() {
		p, err := scanSSHNodeProvision(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("store: scan ssh node provision row: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate ssh node provision rows: %w", err)
	}
	return out, nil
}

// UpdateSSHNodeProvisionProgress replaces status, detectedOS,
// detectedArch, log, nodeID and failureReason for an existing row: every
// field the provisioning goroutine (as the install proceeds) or a GET's
// live enrollment check (once it finds the matching node) can learn
// between one write and the next. Returns ErrSSHNodeProvisionNotFound if
// id doesn't exist.
func (db *DB) UpdateSSHNodeProvisionProgress(ctx context.Context, id, status, detectedOS, detectedArch, log, nodeID, failureReason string, updatedAt time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE ssh_node_provisions
		SET status = ?, detected_os = ?, detected_arch = ?, log = ?, node_id = ?, failure_reason = ?, updated_at = ?
		WHERE id = ?
	`, status, detectedOS, detectedArch, log, nodeID, failureReason, formatTime(updatedAt), id)
	if err != nil {
		return fmt.Errorf("store: update ssh node provision %q: %w", id, err)
	}
	return rowsAffectedOrNotFound(res, ErrSSHNodeProvisionNotFound, "update ssh node provision %q", id)
}

func scanSSHNodeProvision(scan func(dest ...any) error) (SSHNodeProvision, error) {
	var (
		p                    SSHNodeProvision
		createdAt, updatedAt string
	)
	if err := scan(&p.ID, &p.Name, &p.Role, &p.Status, &p.DetectedOS, &p.DetectedArch,
		&p.NodeID, &p.FailureReason, &p.Log, &createdAt, &updatedAt); err != nil {
		return SSHNodeProvision{}, err
	}
	var err error
	if p.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return SSHNodeProvision{}, fmt.Errorf("parse created_at: %w", err)
	}
	if p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return SSHNodeProvision{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return p, nil
}
