package api

import (
	"errors"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

// NodeSessionCloser ends a node's live agent session. *agent.Server
// satisfies it; nil means a revoked node is only refused at its next
// connection.
type NodeSessionCloser interface {
	Disconnect(nodeID string)
}

// nodeCertConfig holds the certificate and agent version settings the node
// routes report against (ADR 021).
type nodeCertConfig struct {
	warning, critical time.Duration
	minAgentVersion   string
	sessions          NodeSessionCloser
}

// WithNodeCertThresholds sets when a node's agent certificate is reported
// as expiring (warning) and critical. Zero keeps the alerting defaults.
func WithNodeCertThresholds(warning, critical time.Duration) Option {
	return func(rt *Router) { rt.nodeCerts.warning, rt.nodeCerts.critical = warning, critical }
}

// WithMinAgentVersion flags nodes whose agent reports an older version.
// Empty disables the check.
func WithMinAgentVersion(v string) Option {
	return func(rt *Router) { rt.nodeCerts.minAgentVersion = v }
}

// WithNodeSessionCloser lets certificate revocation close the node's live
// session immediately.
func WithNodeSessionCloser(c NodeSessionCloser) Option {
	return func(rt *Router) { rt.nodeCerts.sessions = c }
}

func (c nodeCertConfig) thresholds() (warning, critical time.Duration) {
	warning, critical = c.warning, c.critical
	if warning <= 0 {
		warning = alerting.DefaultNodeCertWarning
	}
	if critical <= 0 {
		critical = alerting.DefaultNodeCertCritical
	}
	return warning, critical
}

// nodeCertResource is a node's agent certificate state.
type nodeCertResource struct {
	// State is ok, expiring, critical, expired, revoked, or unknown.
	State         string     `json:"state"`
	NotAfter      *time.Time `json:"not_after,omitempty"`
	DaysRemaining *int       `json:"days_remaining,omitempty"`
	RenewedAt     *time.Time `json:"renewed_at,omitempty"`
	Generation    int        `json:"generation"`
	// KeyOrigin is "agent" when the node generated its own key, "server"
	// for nodes enrolled before that; they switch at their next renewal.
	KeyOrigin          string     `json:"key_origin"`
	PreviousValidUntil *time.Time `json:"previous_valid_until,omitempty"`
	RevokedAt          *time.Time `json:"revoked_at,omitempty"`
	WarningDays        int        `json:"warning_days"`
	CriticalDays       int        `json:"critical_days"`
}

// nodeAgentResource is what the node's agent reported about itself.
type nodeAgentResource struct {
	Version    string     `json:"version,omitempty"`
	Commit     string     `json:"commit,omitempty"`
	OS         string     `json:"os,omitempty"`
	Arch       string     `json:"arch,omitempty"`
	ReportedAt *time.Time `json:"reported_at,omitempty"`
	// Outdated is true when the agent is older than MinVersion, or has
	// never reported a version while a minimum is set.
	Outdated            bool   `json:"outdated"`
	MinVersion          string `json:"min_version,omitempty"`
	ControlPlaneVersion string `json:"control_plane_version"`
}

func (rt *Router) nodeCertAndAgent(n store.Node, now time.Time) (*nodeCertResource, *nodeAgentResource) {
	warning, critical := rt.nodeCerts.thresholds()
	state, left := alerting.ClassifyNodeCert(n, warning, critical, now)
	cert := &nodeCertResource{
		State: string(state), NotAfter: n.CertNotAfter, RenewedAt: n.CertRenewedAt, Generation: n.CertGeneration,
		KeyOrigin: n.CertKeyOrigin, RevokedAt: n.CertRevokedAt,
		WarningDays: int(warning.Hours() / 24), CriticalDays: int(critical.Hours() / 24),
	}
	if n.CertNotAfter != nil && state != alerting.NodeCertRevoked {
		days := int(math.Floor(left.Hours() / 24))
		cert.DaysRemaining = &days
	}
	if n.PrevCertValidUntil != nil && now.Before(*n.PrevCertValidUntil) {
		cert.PreviousValidUntil = n.PrevCertValidUntil
	}

	ag := &nodeAgentResource{
		Version: n.AgentVersion, Commit: n.AgentCommit, OS: n.AgentOS, Arch: n.AgentArch, ReportedAt: n.AgentReportedAt,
		MinVersion: rt.nodeCerts.minAgentVersion, ControlPlaneVersion: version.Version,
	}
	if ag.MinVersion != "" {
		cmp, ok := version.Compare(n.AgentVersion, ag.MinVersion)
		ag.Outdated = n.AgentVersion == "" || (ok && cmp < 0)
	}
	return cert, ag
}

// reenrollTokenResponse is a re-enrollment token's only appearance in
// plaintext.
type reenrollTokenResponse struct {
	Token         string    `json:"token"`
	NodeID        string    `json:"node_id"`
	ExpiresAt     time.Time `json:"expires_at"`
	CAFingerprint string    `json:"ca_fingerprint,omitempty"`
}

// handleCreateNodeReenrollToken handles POST /api/v1/nodes/{id}/reenroll-token:
// mints a single-use token that lets this node obtain a new certificate
// while keeping its identity, for a node offline past expiry or revoked.
func (rt *Router) handleCreateNodeReenrollToken(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: reenroll token: look up node failed", slog.String("node_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	plaintext, err := randomToken()
	if err != nil {
		rt.logger.Error("api: reenroll token: generate token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	tokenID, err := randomNodeJoinTokenID()
	if err != nil {
		rt.logger.Error("api: reenroll token: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	now := time.Now()
	rec := store.NodeJoinToken{
		ID: tokenID, TokenHash: hashToken(plaintext), CreatedAt: now, ExpiresAt: now.Add(nodeJoinTokenTTL),
		Purpose: store.NodeJoinTokenPurposeReenroll, NodeID: id,
	}
	if err := rt.nodes.SaveNodeJoinToken(r.Context(), rec); err != nil {
		rt.logger.Error("api: reenroll token: save failed", slog.String("node_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	rt.logger.Info("api: node re-enrollment token issued", slog.String("node_id", id))
	writeJSON(w, http.StatusCreated, reenrollTokenResponse{Token: plaintext, NodeID: id, ExpiresAt: rec.ExpiresAt, CAFingerprint: rt.agentCAFingerprint})
}

// handleRevokeNodeCert handles POST /api/v1/nodes/{id}/revoke-cert: the
// node's certificate is refused from now on and its live session closed.
// Only a re-enrollment token brings it back.
func (rt *Router) handleRevokeNodeCert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := rt.nodes.RevokeNodeCert(r.Context(), id, time.Now()); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	} else if err != nil {
		rt.logger.Error("api: revoke node cert failed", slog.String("node_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rt.nodeCerts.sessions != nil {
		rt.nodeCerts.sessions.Disconnect(id)
	}
	rt.logger.Warn("api: node certificate revoked", slog.String("node_id", id))
	n, err := rt.nodes.GetNode(r.Context(), id)
	if err != nil {
		rt.logger.Error("api: revoke node cert: reload node failed", slog.String("node_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	res := toNodeResource(*n)
	res.IsLocal = n.ID == rt.localNodeID
	res.Cert, res.Agent = rt.nodeCertAndAgent(*n, time.Now())
	writeJSON(w, http.StatusOK, res)
}

// nodeResourceFromPath is the IAM resource for a node route: "node:<id>".
func nodeResourceFromPath(r *http.Request) string {
	return "node:" + r.PathValue("id")
}
