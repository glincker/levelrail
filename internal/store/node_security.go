package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Node security event kinds.
const (
	SecurityKindEnroll       = "enroll"
	SecurityKindReenroll     = "reenroll"
	SecurityKindCAPin        = "ca_pin"
	SecurityKindConnect      = "connect"
	SecurityKindCertIssued   = "cert_issued"
	SecurityKindRenewFailed  = "renew_failed"
	SecurityKindRevoke       = "revoke"
	SecurityKindRenewRequest = "renew_requested"
)

// Node security event outcomes.
const (
	SecurityOutcomeAccepted = "accepted"
	SecurityOutcomeRejected = "rejected"
	SecurityOutcomeWarning  = "warning"
	SecurityOutcomeFailed   = "failed"
	SecurityOutcomeInfo     = "info"
)

// Node security event reasons. The UI and CLI map these to wording.
const (
	SecurityReasonVerified            = "mutual_tls_verified"
	SecurityReasonExpired             = "expired"
	SecurityReasonNotYetValid         = "not_yet_valid"
	SecurityReasonUnknownCA           = "unknown_ca"
	SecurityReasonInvalidCert         = "invalid_certificate"
	SecurityReasonRevoked             = "revoked"
	SecurityReasonFingerprintMismatch = "fingerprint_mismatch"
	SecurityReasonUnknownNode         = "unknown_node"
	SecurityReasonNoCertificate       = "no_certificate"
	SecurityReasonPinned              = "pinned_verified"
	SecurityReasonTrustedCA           = "trusted_ca"
	SecurityReasonNotPinned           = "not_pinned"
	SecurityReasonNotReported         = "not_reported"
	SecurityReasonEnrollRefused       = "enroll_refused"
	SecurityReasonSignFailed          = "sign_failed"
	SecurityReasonRenewConflict       = "renewal_conflict"
	SecurityReasonUpgradeRequired     = "agent_upgrade_required"
	SecurityReasonReenrollRequired    = "reenroll_required"
)

// CA pin states an agent reports at enrollment.
const (
	CAPinPinned    = "pinned"
	CAPinTrustedCA = "trusted_ca"
	CAPinUnpinned  = "unpinned"
)

// NodeSecurityDetail is the small, fixed set of facts an event may carry.
// It never holds key material or a join token, only the token's id.
type NodeSecurityDetail struct {
	TokenID     string `json:"token_id,omitempty"`
	Subject     string `json:"subject,omitempty"`
	Message     string `json:"message,omitempty"`
	Serial      string `json:"serial,omitempty"`
	NotBefore   int64  `json:"not_before,omitempty"`
	NotAfter    int64  `json:"not_after,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Actor       string `json:"actor,omitempty"`
}

// NodeSecurityEvent is one row of the node security log. Count is above one
// when identical events were coalesced; FirstAt and LastAt bound them.
type NodeSecurityEvent struct {
	ID       int64
	NodeID   string
	Kind     string
	Outcome  string
	Reason   string
	SourceIP string
	Detail   NodeSecurityDetail
	Count    int
	FirstAt  time.Time
	LastAt   time.Time
}

// SecurityEventPolicy bounds the log. Coalesce merges an identical event
// seen again inside Window into the existing row.
type SecurityEventPolicy struct {
	Coalesce   bool
	Window     time.Duration
	PerNodeCap int
}

// AddNodeSecurityEvent appends ev, or when p.Coalesce is set and an event
// with the same node, kind, outcome, reason and source exists inside
// p.Window, bumps that row's count instead. The node's rows are then cut
// to p.PerNodeCap, newest kept.
func (db *DB) AddNodeSecurityEvent(ctx context.Context, ev NodeSecurityEvent, p SecurityEventPolicy, now time.Time) error {
	detail, err := json.Marshal(ev.Detail)
	if err != nil {
		return fmt.Errorf("store: encode security event detail: %w", err)
	}
	nowMS := now.UnixMilli()
	if p.Coalesce && p.Window > 0 {
		res, err := db.ExecContext(ctx, `
			UPDATE node_security_events SET count = count + ?, last_at = ?, detail = ?
			WHERE id = (
				SELECT id FROM node_security_events
				WHERE node_id = ? AND kind = ? AND outcome = ? AND reason = ? AND source_ip = ? AND last_at >= ?
				ORDER BY last_at DESC LIMIT 1)
		`, max(ev.Count, 1), nowMS, string(detail), ev.NodeID, ev.Kind, ev.Outcome, ev.Reason, ev.SourceIP, now.Add(-p.Window).UnixMilli())
		if err != nil {
			return fmt.Errorf("store: coalesce security event: %w", err)
		}
		if n, _ := res.RowsAffected(); n > 0 {
			return nil
		}
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO node_security_events (node_id, kind, outcome, reason, source_ip, detail, count, first_at, last_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ev.NodeID, ev.Kind, ev.Outcome, ev.Reason, ev.SourceIP, string(detail), max(ev.Count, 1), nowMS, nowMS); err != nil {
		return fmt.Errorf("store: insert security event: %w", err)
	}
	if p.PerNodeCap > 0 {
		if _, err := db.ExecContext(ctx, `
			DELETE FROM node_security_events WHERE node_id = ? AND id NOT IN (
				SELECT id FROM node_security_events WHERE node_id = ? ORDER BY last_at DESC, id DESC LIMIT ?)
		`, ev.NodeID, ev.NodeID, p.PerNodeCap); err != nil {
			return fmt.Errorf("store: trim security events: %w", err)
		}
	}
	return nil
}

// PruneNodeSecurityEvents deletes events last seen before cutoff and
// returns how many went.
func (db *DB) PruneNodeSecurityEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := db.ExecContext(ctx, `DELETE FROM node_security_events WHERE last_at < ?`, cutoff.UnixMilli())
	if err != nil {
		return 0, fmt.Errorf("store: prune security events: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// NodeSecurityFilter selects events for ListNodeSecurityEvents.
type NodeSecurityFilter struct {
	Outcome string
	Limit   int
}

// ListNodeSecurityEvents returns nodeID's events, newest first.
func (db *DB) ListNodeSecurityEvents(ctx context.Context, nodeID string, f NodeSecurityFilter) ([]NodeSecurityEvent, error) {
	q := `SELECT id, node_id, kind, outcome, reason, source_ip, detail, count, first_at, last_at
		FROM node_security_events WHERE node_id = ?`
	args := []any{nodeID}
	if f.Outcome != "" {
		q += ` AND outcome = ?`
		args = append(args, f.Outcome)
	}
	q += ` ORDER BY last_at DESC, id DESC LIMIT ?`
	args = append(args, max(f.Limit, 1))
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("store: list security events for node %q: %w", nodeID, err)
	}
	defer func() { _ = rows.Close() }()
	var out []NodeSecurityEvent
	for rows.Next() {
		var (
			e           NodeSecurityEvent
			detail      string
			first, last int64
		)
		if err := rows.Scan(&e.ID, &e.NodeID, &e.Kind, &e.Outcome, &e.Reason, &e.SourceIP, &detail, &e.Count, &first, &last); err != nil {
			return nil, fmt.Errorf("store: scan security event: %w", err)
		}
		if err := json.Unmarshal([]byte(detail), &e.Detail); err != nil {
			return nil, fmt.Errorf("store: decode security event detail: %w", err)
		}
		e.FirstAt, e.LastAt = time.UnixMilli(first).UTC(), time.UnixMilli(last).UTC()
		out = append(out, e)
	}
	return out, rows.Err()
}

// NodeConnectSummary counts a node's connection verifications since a time,
// weighting coalesced rows by their count.
type NodeConnectSummary struct {
	Accepted, Rejected int
	LastAccepted       *time.Time
	LastRejected       *time.Time
}

// SummarizeNodeConnections counts kind=connect events for nodeID since.
func (db *DB) SummarizeNodeConnections(ctx context.Context, nodeID string, since time.Time) (NodeConnectSummary, error) {
	var s NodeConnectSummary
	rows, err := db.QueryContext(ctx, `
		SELECT outcome, COALESCE(SUM(count), 0), MAX(last_at) FROM node_security_events
		WHERE node_id = ? AND kind = ? AND last_at >= ? GROUP BY outcome
	`, nodeID, SecurityKindConnect, since.UnixMilli())
	if err != nil {
		return s, fmt.Errorf("store: summarize security events for node %q: %w", nodeID, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var (
			outcome string
			n       int
			last    int64
		)
		if err := rows.Scan(&outcome, &n, &last); err != nil {
			return s, fmt.Errorf("store: scan security summary: %w", err)
		}
		t := time.UnixMilli(last).UTC()
		switch outcome {
		case SecurityOutcomeAccepted:
			s.Accepted, s.LastAccepted = n, &t
		case SecurityOutcomeRejected:
			s.Rejected, s.LastRejected = n, &t
		}
	}
	return s, rows.Err()
}

// NodeTrustState is what the control plane knows of a node's trust setup
// beyond the certificate columns: the CA pin the agent reported and its
// renewal schedule and failures.
type NodeTrustState struct {
	NodeID                string
	CAPinState            string
	CAPinAt               *time.Time
	RenewNextAt           *time.Time
	RenewLastError        string
	RenewFailedAt         *time.Time
	RenewNextRetryAt      *time.Time
	RenewConsecutiveFails int
	RenewReportedAt       *time.Time
}

func msPtr(ms sql.NullInt64) *time.Time {
	if !ms.Valid {
		return nil
	}
	t := time.UnixMilli(ms.Int64).UTC()
	return &t
}

func nullMS(t *time.Time) sql.NullInt64 {
	if t == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: t.UnixMilli(), Valid: true}
}

// GetNodeTrustState returns the node's trust state, or the zero value with
// the node id set when nothing was recorded yet.
func (db *DB) GetNodeTrustState(ctx context.Context, nodeID string) (NodeTrustState, error) {
	s := NodeTrustState{NodeID: nodeID}
	var pinAt, next, failed, retry, reported sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT ca_pin_state, ca_pin_at, renew_next_at, renew_last_error, renew_failed_at,
			renew_next_retry_at, renew_consecutive_fails, renew_reported_at
		FROM node_trust_state WHERE node_id = ?`, nodeID).
		Scan(&s.CAPinState, &pinAt, &next, &s.RenewLastError, &failed, &retry, &s.RenewConsecutiveFails, &reported)
	if errors.Is(err, sql.ErrNoRows) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("store: get trust state for node %q: %w", nodeID, err)
	}
	s.CAPinAt, s.RenewNextAt, s.RenewFailedAt = msPtr(pinAt), msPtr(next), msPtr(failed)
	s.RenewNextRetryAt, s.RenewReportedAt = msPtr(retry), msPtr(reported)
	return s, nil
}

// ListNodeRenewalStates returns every node's stored renewal report in one
// query, keyed by node id, for list paths that must not read per node.
func (db *DB) ListNodeRenewalStates(ctx context.Context) (map[string]NodeTrustState, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT node_id, renew_last_error, renew_failed_at, renew_next_retry_at, renew_consecutive_fails
		FROM node_trust_state WHERE renew_last_error != '' OR renew_consecutive_fails > 0`)
	if err != nil {
		return nil, fmt.Errorf("store: list node renewal states: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]NodeTrustState{}
	for rows.Next() {
		var s NodeTrustState
		var failed, retry sql.NullInt64
		if err := rows.Scan(&s.NodeID, &s.RenewLastError, &failed, &retry, &s.RenewConsecutiveFails); err != nil {
			return nil, fmt.Errorf("store: scan node renewal state: %w", err)
		}
		s.RenewFailedAt, s.RenewNextRetryAt = msPtr(failed), msPtr(retry)
		out[s.NodeID] = s
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: list node renewal states: %w", err)
	}
	return out, nil
}

// SetNodeCAPin records the CA pin state an agent reported at enrollment.
func (db *DB) SetNodeCAPin(ctx context.Context, nodeID, state string, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_trust_state (node_id, ca_pin_state, ca_pin_at) VALUES (?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET ca_pin_state = excluded.ca_pin_state, ca_pin_at = excluded.ca_pin_at
	`, nodeID, state, now.UnixMilli())
	if err != nil {
		return fmt.Errorf("store: set CA pin for node %q: %w", nodeID, err)
	}
	return nil
}

// SetNodeRenewalReport stores the agent's latest renewal schedule and
// failure state.
func (db *DB) SetNodeRenewalReport(ctx context.Context, s NodeTrustState, now time.Time) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO node_trust_state (node_id, renew_next_at, renew_last_error, renew_failed_at,
			renew_next_retry_at, renew_consecutive_fails, renew_reported_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(node_id) DO UPDATE SET renew_next_at = excluded.renew_next_at,
			renew_last_error = excluded.renew_last_error, renew_failed_at = excluded.renew_failed_at,
			renew_next_retry_at = excluded.renew_next_retry_at,
			renew_consecutive_fails = excluded.renew_consecutive_fails,
			renew_reported_at = excluded.renew_reported_at
	`, s.NodeID, nullMS(s.RenewNextAt), s.RenewLastError, nullMS(s.RenewFailedAt), nullMS(s.RenewNextRetryAt),
		s.RenewConsecutiveFails, now.UnixMilli())
	if err != nil {
		return fmt.Errorf("store: set renewal report for node %q: %w", s.NodeID, err)
	}
	return nil
}

func (db *DB) deleteNodeSecurityData(ctx context.Context, id string) error {
	for _, q := range []string{
		`DELETE FROM node_security_events WHERE node_id = ?`,
		`DELETE FROM node_trust_state WHERE node_id = ?`,
	} {
		if _, err := db.ExecContext(ctx, q, id); err != nil {
			return fmt.Errorf("store: delete security data for node %q: %w", id, err)
		}
	}
	return nil
}
