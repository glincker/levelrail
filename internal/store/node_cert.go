package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Certificate key origins: who generated the private key behind a node's
// current certificate.
const (
	CertKeyOriginServer = "server"
	CertKeyOriginAgent  = "agent"
)

// ErrNodeCertNotAccepted is returned when the presented certificate is
// neither the node's current one nor its previous one inside the overlap
// window.
var ErrNodeCertNotAccepted = errors.New("store: certificate not accepted for this node")

// ErrNodeCertRevoked is returned when the node's certificate was revoked
// by an operator.
var ErrNodeCertRevoked = errors.New("store: node certificate revoked")

// ErrNodeCertConflict is returned when the node's certificate changed
// between read and write, a concurrent renewal having won.
var ErrNodeCertConflict = errors.New("store: node certificate changed concurrently")

// NodeCert is a newly issued certificate's identifying data.
type NodeCert struct {
	Fingerprint string
	Serial      string
	NotAfter    time.Time
	KeyOrigin   string
}

// NodeAgentInfo is what an agent reports about its own build and platform.
type NodeAgentInfo struct {
	Version string
	Commit  string
	OS      string
	Arch    string
}

// AcceptsCert reports whether fingerprint may authenticate as n at now:
// the current certificate, or the previous one inside its overlap window,
// and never while revoked.
func (n Node) AcceptsCert(fingerprint string, now time.Time) bool {
	if fingerprint == "" || n.CertRevokedAt != nil {
		return false
	}
	if fingerprint == n.CertFingerprint {
		return true
	}
	return fingerprint == n.PrevCertFingerprint && n.PrevCertValidUntil != nil && now.Before(*n.PrevCertValidUntil)
}

// RotateNodeCert records a renewal authenticated by presentedFingerprint.
// The presented certificate becomes the previous one, accepted until
// now+grace; a retry presenting the already-previous certificate keeps
// its original window rather than extending it.
func (db *DB) RotateNodeCert(ctx context.Context, id, presentedFingerprint string, c NodeCert, grace time.Duration, now time.Time) error {
	n, err := db.GetNode(ctx, id)
	if err != nil {
		return err
	}
	if n.CertRevokedAt != nil {
		return ErrNodeCertRevoked
	}
	if !n.AcceptsCert(presentedFingerprint, now) {
		return ErrNodeCertNotAccepted
	}
	prevUntil := now.Add(grace)
	if presentedFingerprint != n.CertFingerprint && n.PrevCertValidUntil != nil {
		prevUntil = *n.PrevCertValidUntil
	}
	res, err := db.ExecContext(ctx, `
		UPDATE nodes SET
			prev_cert_fingerprint = ?, prev_cert_valid_until = ?,
			cert_fingerprint = ?, cert_serial = ?, cert_not_after = ?, cert_key_origin = ?,
			cert_renewed_at = ?, cert_generation = cert_generation + 1,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ? AND cert_fingerprint = ? AND cert_revoked_at IS NULL
	`, presentedFingerprint, formatTime(prevUntil),
		c.Fingerprint, c.Serial, formatTime(c.NotAfter), certKeyOriginOrDefault(c.KeyOrigin),
		formatTime(now), id, n.CertFingerprint)
	if err != nil {
		return fmt.Errorf("store: rotate cert for node %q: %w", id, err)
	}
	return requireOneRow(res, ErrNodeCertConflict, "rotate cert for node "+id)
}

// ReenrollNodeCert installs a certificate issued through a re-enrollment
// token: the previous certificate is dropped and any revocation cleared,
// since minting the token was the operator's authorization.
func (db *DB) ReenrollNodeCert(ctx context.Context, id string, c NodeCert, now time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE nodes SET
			prev_cert_fingerprint = '', prev_cert_valid_until = NULL, cert_revoked_at = NULL,
			cert_fingerprint = ?, cert_serial = ?, cert_not_after = ?, cert_key_origin = ?,
			cert_renewed_at = ?, cert_generation = cert_generation + 1,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, c.Fingerprint, c.Serial, formatTime(c.NotAfter), certKeyOriginOrDefault(c.KeyOrigin), formatTime(now), id)
	if err != nil {
		return fmt.Errorf("store: re-enroll cert for node %q: %w", id, err)
	}
	return requireOneRow(res, ErrNodeNotFound, "re-enroll cert for node "+id)
}

// RevokeNodeCert marks the node's certificate revoked. Session and renewal
// refuse it from then on; only a re-enrollment token brings the node back.
func (db *DB) RevokeNodeCert(ctx context.Context, id string, now time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE nodes SET cert_revoked_at = ?, prev_cert_fingerprint = '', prev_cert_valid_until = NULL,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?
	`, formatTime(now), id)
	if err != nil {
		return fmt.Errorf("store: revoke cert for node %q: %w", id, err)
	}
	return requireOneRow(res, ErrNodeNotFound, "revoke cert for node "+id)
}

// SyncNodeCertDetails records the real expiry and serial of the node's
// current certificate, replacing the estimate migration 0135 backfilled.
// A no-op when fingerprint is not the current certificate.
func (db *DB) SyncNodeCertDetails(ctx context.Context, id, fingerprint, serial string, notAfter time.Time) error {
	_, err := db.ExecContext(ctx, `
		UPDATE nodes SET cert_not_after = ?, cert_serial = ?
		WHERE id = ? AND cert_fingerprint = ? AND (cert_not_after IS NOT ? OR cert_serial != ?)
	`, formatTime(notAfter), serial, id, fingerprint, formatTime(notAfter), serial)
	if err != nil {
		return fmt.Errorf("store: sync cert details for node %q: %w", id, err)
	}
	return nil
}

// UpdateNodeAgentInfo stores what the node's agent reported about itself.
func (db *DB) UpdateNodeAgentInfo(ctx context.Context, id string, info NodeAgentInfo, now time.Time) error {
	res, err := db.ExecContext(ctx, `
		UPDATE nodes SET agent_version = ?, agent_commit = ?, agent_os = ?, agent_arch = ?, agent_reported_at = ?
		WHERE id = ?
	`, info.Version, info.Commit, info.OS, info.Arch, formatTime(now), id)
	if err != nil {
		return fmt.Errorf("store: update agent info for node %q: %w", id, err)
	}
	return requireOneRow(res, ErrNodeNotFound, "update agent info for node "+id)
}

func requireOneRow(res sql.Result, notFound error, op string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: %s: rows affected: %w", op, err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}

func certKeyOriginOrDefault(origin string) string {
	if origin == "" {
		return CertKeyOriginServer
	}
	return origin
}

// nodeCertTimeCount is the number of nullable timestamp columns scanNode
// reads for certificate and agent state, in parseNodeCertTimes' order.
const nodeCertTimeCount = 5

func parseNodeCertTimes(n *Node, raw [nodeCertTimeCount]sql.NullString) error {
	targets := [nodeCertTimeCount]**time.Time{&n.CertNotAfter, &n.CertRenewedAt, &n.PrevCertValidUntil, &n.CertRevokedAt, &n.AgentReportedAt}
	names := [nodeCertTimeCount]string{"cert_not_after", "cert_renewed_at", "prev_cert_valid_until", "cert_revoked_at", "agent_reported_at"}
	for i := range raw {
		t, err := parseTimePtr(raw[i])
		if err != nil {
			return fmt.Errorf("parse %s: %w", names[i], err)
		}
		*targets[i] = t
	}
	return nil
}
