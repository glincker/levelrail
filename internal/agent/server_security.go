package agent

// This file: the control plane's record of what it verified, and when.
// Every call here is best effort: a failed write is logged, never a reason
// to refuse or fail the RPC that triggered it.

import (
	"context"
	"crypto/x509"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ErrNodeNotConnected is returned by RequestRenew when the node has no
// live session to carry the request.
var ErrNodeNotConnected = errors.New("agent: node is not connected")

// WithSecurityLog turns on the per-node security event log, stored in st.
func WithSecurityLog(st SecurityStore, cfg SecurityConfig) Option {
	return func(s *Server) { s.secStore, s.secCfg = st, cfg }
}

func (s *Server) initSecurity() {
	s.caPool = x509.NewCertPool()
	s.caPool.AddCert(s.ca.cert)
	if s.secStore == nil {
		return
	}
	s.sec = newSecurityRecorder(s.secStore, s.secCfg, s.logger, s.now, func(ctx context.Context, subject string) bool {
		_, err := s.store.GetNode(ctx, subject)
		return err == nil
	})
}

// ObserveRejected records a client certificate the TLS handshake refused.
func (s *Server) ObserveRejected(r RejectedHandshake) {
	s.sec.record(store.NodeSecurityEvent{
		Kind: store.SecurityKindConnect, Outcome: store.SecurityOutcomeRejected, Reason: r.Reason, SourceIP: r.SourceIP,
		Detail: store.NodeSecurityDetail{Subject: r.Subject, Fingerprint: r.Fingerprint},
	}, true)
}

// RecordSecurityEvent appends an event without coalescing, for operator
// actions such as revoking a certificate.
func (s *Server) RecordSecurityEvent(ev store.NodeSecurityEvent) { s.sec.record(ev, false) }

// RequestRenew asks nodeID's agent, over its live session, to renew its
// certificate now.
func (s *Server) RequestRenew(nodeID string) error {
	s.sessionsMu.Lock()
	live, ok := s.sessions[nodeID]
	s.sessionsMu.Unlock()
	if !ok {
		return ErrNodeNotConnected
	}
	return live.m.sendFrame(&agentpb.ControlMessage{Payload: &agentpb.ControlMessage_RenewNow{RenewNow: &agentpb.RenewNow{}}})
}

func (s *Server) recordRejected(ctx context.Context, nodeID, subject, reason, fingerprint string) {
	s.sec.record(store.NodeSecurityEvent{
		NodeID: nodeID, Kind: store.SecurityKindConnect, Outcome: store.SecurityOutcomeRejected, Reason: reason, SourceIP: peerIP(ctx),
		Detail: store.NodeSecurityDetail{Subject: subject, Fingerprint: fingerprint},
	}, true)
}

func (s *Server) recordAccepted(ctx context.Context, nodeID string, cert *x509.Certificate) {
	s.sec.record(store.NodeSecurityEvent{
		NodeID: nodeID, Kind: store.SecurityKindConnect, Outcome: store.SecurityOutcomeAccepted, Reason: store.SecurityReasonVerified, SourceIP: peerIP(ctx),
		Detail: store.NodeSecurityDetail{Fingerprint: CertFingerprint(cert.Raw), Serial: certSerial(cert)},
	}, true)
}

func (s *Server) recordTokenRefusal(ctx context.Context, kind, nodeID string, err error) {
	s.sec.record(store.NodeSecurityEvent{
		NodeID: nodeID, Kind: kind, Outcome: store.SecurityOutcomeRejected, Reason: store.SecurityReasonEnrollRefused, SourceIP: peerIP(ctx),
		Detail: store.NodeSecurityDetail{Message: status.Convert(err).Message()},
	}, true)
}

// errAgentUpgradeRequired is what an agent that cannot send a CSR sees. It
// is static: nothing about the token or the node leaks into it.
var errAgentUpgradeRequired = status.Error(codes.FailedPrecondition,
	"this agent is too old to enroll: it must generate its own key. Upgrade the agent, then enroll again")

func (s *Server) recordUpgradeRequired(ctx context.Context, kind, nodeID string) {
	s.sec.record(store.NodeSecurityEvent{
		NodeID: nodeID, Kind: kind, Outcome: store.SecurityOutcomeRejected, Reason: store.SecurityReasonUpgradeRequired, SourceIP: peerIP(ctx),
		Detail: store.NodeSecurityDetail{Message: "no certificate signing request: agent must be upgraded"},
	}, true)
}

func certDetail(c IssuedCert) store.NodeSecurityDetail {
	return store.NodeSecurityDetail{Serial: c.Serial, Fingerprint: c.Fingerprint, NotBefore: c.NotBefore.Unix(), NotAfter: c.NotAfter.Unix()}
}

func (s *Server) recordCertIssued(ctx context.Context, nodeID, via string, c IssuedCert) {
	d := certDetail(c)
	s.sec.record(store.NodeSecurityEvent{
		NodeID: nodeID, Kind: store.SecurityKindCertIssued, Outcome: store.SecurityOutcomeInfo, Reason: via, SourceIP: peerIP(ctx), Detail: d,
	}, false)
}

func (s *Server) recordRenewFailed(ctx context.Context, nodeID, reason string) {
	s.sec.record(store.NodeSecurityEvent{
		NodeID: nodeID, Kind: store.SecurityKindRenewFailed, Outcome: store.SecurityOutcomeFailed, Reason: reason, SourceIP: peerIP(ctx),
	}, true)
}

// recordEnrolled logs an enrollment or re-enrollment, the certificate it
// issued, and the CA pin the agent says it checked first. The join token
// appears only as its id.
func (s *Server) recordEnrolled(ctx context.Context, kind, nodeID, tokenID, pinState string, c IssuedCert) {
	ip := peerIP(ctx)
	s.sec.record(store.NodeSecurityEvent{
		NodeID: nodeID, Kind: kind, Outcome: store.SecurityOutcomeAccepted, SourceIP: ip,
		Detail: store.NodeSecurityDetail{TokenID: tokenID},
	}, false)
	s.recordCertIssued(ctx, nodeID, kind, c)

	outcome, reason := store.SecurityOutcomeWarning, store.SecurityReasonNotReported
	switch pinState {
	case store.CAPinPinned:
		outcome, reason = store.SecurityOutcomeAccepted, store.SecurityReasonPinned
	case store.CAPinTrustedCA:
		outcome, reason = store.SecurityOutcomeAccepted, store.SecurityReasonTrustedCA
	case store.CAPinUnpinned:
		outcome, reason = store.SecurityOutcomeWarning, store.SecurityReasonNotPinned
	}
	s.sec.record(store.NodeSecurityEvent{NodeID: nodeID, Kind: store.SecurityKindCAPin, Outcome: outcome, Reason: reason, SourceIP: ip}, false)
	if s.secStore != nil && pinState != store.CAPinTrustedCA {
		if err := s.secStore.SetNodeCAPin(ctx, nodeID, pinState, s.now()); err != nil {
			s.logger.Warn("agent: record CA pin state failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		}
	}
}

func (s *Server) recordRenewalReport(ctx context.Context, nodeID string, r *agentpb.RenewalReport) {
	if s.secStore == nil || r == nil {
		return
	}
	st := store.NodeTrustState{NodeID: nodeID, RenewLastError: cleanText(r.GetLastError(), securityMessageMax), RenewConsecutiveFails: int(r.GetConsecutiveFailures())}
	if t := r.GetNextRenewAt(); t != nil {
		v := t.AsTime()
		st.RenewNextAt = &v
	}
	if t := r.GetLastFailedAt(); t != nil {
		v := t.AsTime()
		st.RenewFailedAt = &v
	}
	if t := r.GetNextRetryAt(); t != nil {
		v := t.AsTime()
		st.RenewNextRetryAt = &v
	}
	if err := s.secStore.SetNodeRenewalReport(ctx, st, s.now()); err != nil {
		s.logger.Warn("agent: store renewal report failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
	}
}
