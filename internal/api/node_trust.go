package api

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultSecurityEventsLimit = 100
	maxSecurityEventsLimit     = 500
	trustWindow                = 24 * time.Hour
)

// NodeTrustControl is the live-agent surface the trust routes need.
// *agent.Server satisfies it.
type NodeTrustControl interface {
	RequestRenew(nodeID string) error
	RecordSecurityEvent(ev store.NodeSecurityEvent)
}

// WithNodeTrustControl wires renew-now and operator security events.
func WithNodeTrustControl(c NodeTrustControl) Option {
	return func(rt *Router) { rt.nodeCerts.trust = c }
}

// nodeTrustStore is the optional NodeStore extension behind the trust
// routes; *store.DB implements it.
type nodeTrustStore interface {
	ListNodeSecurityEvents(ctx context.Context, nodeID string, f store.NodeSecurityFilter) ([]store.NodeSecurityEvent, error)
	SummarizeNodeConnections(ctx context.Context, nodeID string, since time.Time) (store.NodeConnectSummary, error)
	GetNodeTrustState(ctx context.Context, nodeID string) (store.NodeTrustState, error)
}

// Trust status states for nodeTrustStatus.State.
const (
	trustVerified   = "verified"
	trustRejected   = "rejected"
	trustRevoked    = "revoked"
	trustExpired    = "expired"
	trustIdle       = "idle"
	trustLocal      = "local"
	trustNoActivity = "no_activity"
)

// nodeTrustStatus is computed from recent real events, never fixed text.
type nodeTrustStatus struct {
	State string `json:"state"`
	// Reason is the rejection reason when State is rejected.
	Reason string `json:"reason,omitempty"`
}

type nodeTrustCA struct {
	Fingerprint string `json:"fingerprint,omitempty"`
	// PinState is what the agent reported at enrollment: pinned,
	// trusted_ca, unpinned, or empty when never reported.
	PinState      string     `json:"pin_state,omitempty"`
	PinReportedAt *time.Time `json:"pin_reported_at,omitempty"`
}

type nodeTrustCert struct {
	Fingerprint   string     `json:"fingerprint,omitempty"`
	Serial        string     `json:"serial,omitempty"`
	IssuedAt      *time.Time `json:"issued_at,omitempty"`
	NotAfter      *time.Time `json:"not_after,omitempty"`
	DaysRemaining *int       `json:"days_remaining,omitempty"`
	// LifetimeUsed is 0 to 1: how much of the lifetime has passed.
	LifetimeUsed *float64 `json:"lifetime_used,omitempty"`
	KeyOrigin    string   `json:"key_origin"`
}

type nodeTrustRenewal struct {
	NextRenewAt *time.Time `json:"next_renew_at,omitempty"`
	// NextRenewEstimated is true when the time is computed from the default
	// schedule because the agent has not reported its own.
	NextRenewEstimated  bool       `json:"next_renew_estimated,omitempty"`
	LastRenewedAt       *time.Time `json:"last_renewed_at,omitempty"`
	RenewalCount        int        `json:"renewal_count"`
	LastError           string     `json:"last_error,omitempty"`
	LastFailedAt        *time.Time `json:"last_failed_at,omitempty"`
	NextRetryAt         *time.Time `json:"next_retry_at,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
}

type nodeTrustValidation struct {
	WindowHours    int        `json:"window_hours"`
	Accepted       int        `json:"accepted"`
	Rejected       int        `json:"rejected"`
	LastAcceptedAt *time.Time `json:"last_accepted_at,omitempty"`
	LastRejectedAt *time.Time `json:"last_rejected_at,omitempty"`
}

// nodeTrustResource is GET /api/v1/nodes/{id}/security. A local node has
// only Status and IsLocal: it has no network link to secure.
type nodeTrustResource struct {
	NodeID     string               `json:"node_id"`
	IsLocal    bool                 `json:"is_local"`
	Status     nodeTrustStatus      `json:"status"`
	CA         *nodeTrustCA         `json:"ca,omitempty"`
	Cert       *nodeTrustCert       `json:"certificate,omitempty"`
	Renewal    *nodeTrustRenewal    `json:"renewal,omitempty"`
	Validation *nodeTrustValidation `json:"validation,omitempty"`
}

type nodeSecurityEventResource struct {
	ID          int64     `json:"id"`
	Kind        string    `json:"kind"`
	Outcome     string    `json:"outcome"`
	Reason      string    `json:"reason,omitempty"`
	SourceIP    string    `json:"source_ip,omitempty"`
	Count       int       `json:"count"`
	FirstAt     time.Time `json:"first_at"`
	LastAt      time.Time `json:"last_at"`
	TokenID     string    `json:"token_id,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	Message     string    `json:"message,omitempty"`
	Serial      string    `json:"serial,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	NotBefore   int64     `json:"not_before,omitempty"`
	NotAfter    int64     `json:"not_after,omitempty"`
	Actor       string    `json:"actor,omitempty"`
}

func (rt *Router) trustNode(w http.ResponseWriter, r *http.Request) (*store.Node, nodeTrustStore, bool) {
	ts, ok := rt.nodes.(nodeTrustStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "node security events are not available on this control plane")
		return nil, nil, false
	}
	id := r.PathValue("id")
	n, err := rt.nodes.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return nil, nil, false
	}
	if err != nil {
		rt.internalError(w, "api: node trust: look up node failed", err, slog.String("node_id", id))
		return nil, nil, false
	}
	return n, ts, true
}

// handleGetNodeTrust handles GET /api/v1/nodes/{id}/security.
func (rt *Router) handleGetNodeTrust(w http.ResponseWriter, r *http.Request) {
	n, ts, ok := rt.trustNode(w, r)
	if !ok {
		return
	}
	if rt.isLocalNodeRecord(n) {
		writeJSON(w, http.StatusOK, nodeTrustResource{NodeID: n.ID, IsLocal: true, Status: nodeTrustStatus{State: trustLocal}})
		return
	}
	now := time.Now()
	ctx := r.Context()
	sum, err := ts.SummarizeNodeConnections(ctx, n.ID, now.Add(-trustWindow))
	if err != nil {
		rt.internalError(w, "api: node trust: summarize failed", err, slog.String("node_id", n.ID))
		return
	}
	state, err := ts.GetNodeTrustState(ctx, n.ID)
	if err != nil {
		rt.internalError(w, "api: node trust: load state failed", err, slog.String("node_id", n.ID))
		return
	}
	issued := rt.latestIssuedAt(ctx, ts, n)
	res := nodeTrustResource{
		NodeID:  n.ID,
		CA:      &nodeTrustCA{Fingerprint: rt.agentCAFingerprint, PinState: state.CAPinState, PinReportedAt: state.CAPinAt},
		Cert:    trustCert(n, issued, now),
		Renewal: trustRenewal(n, state, issued),
		Validation: &nodeTrustValidation{
			WindowHours: int(trustWindow.Hours()), Accepted: sum.Accepted, Rejected: sum.Rejected,
			LastAcceptedAt: sum.LastAccepted, LastRejectedAt: sum.LastRejected,
		},
	}
	res.Status = rt.trustStatus(ctx, ts, n, now)
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) isLocalNodeRecord(n *store.Node) bool {
	return rt.localNodeID != "" && n.ID == rt.localNodeID
}

// latestIssuedAt is when the node's current certificate was issued: the
// newest cert_issued event, else the renewal or enrollment time.
func (rt *Router) latestIssuedAt(ctx context.Context, ts nodeTrustStore, n *store.Node) *time.Time {
	evs, err := ts.ListNodeSecurityEvents(ctx, n.ID, store.NodeSecurityFilter{Limit: 20})
	if err == nil {
		for _, e := range evs {
			if e.Kind == store.SecurityKindCertIssued && e.Detail.Fingerprint == n.CertFingerprint {
				t := e.LastAt
				return &t
			}
		}
	}
	if n.CertRenewedAt != nil {
		return n.CertRenewedAt
	}
	t := n.CreatedAt
	return &t
}

func trustCert(n *store.Node, issued *time.Time, now time.Time) *nodeTrustCert {
	c := &nodeTrustCert{Fingerprint: n.CertFingerprint, Serial: n.CertSerial, IssuedAt: issued, NotAfter: n.CertNotAfter, KeyOrigin: n.CertKeyOrigin}
	if n.CertNotAfter == nil {
		return c
	}
	days := int(math.Floor(n.CertNotAfter.Sub(now).Hours() / 24))
	c.DaysRemaining = &days
	if issued != nil {
		if life := n.CertNotAfter.Sub(*issued); life > 0 {
			used := math.Min(1, math.Max(0, float64(now.Sub(*issued))/float64(life)))
			c.LifetimeUsed = &used
		}
	}
	return c
}

func trustRenewal(n *store.Node, st store.NodeTrustState, issued *time.Time) *nodeTrustRenewal {
	r := &nodeTrustRenewal{
		LastRenewedAt: n.CertRenewedAt, RenewalCount: n.CertGeneration, LastError: st.RenewLastError,
		LastFailedAt: st.RenewFailedAt, NextRetryAt: st.RenewNextRetryAt, ConsecutiveFailures: st.RenewConsecutiveFails,
	}
	switch {
	case st.RenewNextAt != nil && !st.RenewNextAt.Before(issuedOrZero(issued)):
		r.NextRenewAt = st.RenewNextAt
	case n.CertNotAfter != nil && issued != nil:
		est := issued.Add(time.Duration(float64(n.CertNotAfter.Sub(*issued)) * agent.DefaultRenewFraction))
		r.NextRenewAt, r.NextRenewEstimated = &est, true
	}
	return r
}

func issuedOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// trustStatus decides the headline from real events: verified only while
// the node is online and its newest accepted connection presented a
// certificate the node record still accepts. A stray rejection (say a
// hostile client naming this node) does not hide a live verified session.
func (rt *Router) trustStatus(ctx context.Context, ts nodeTrustStore, n *store.Node, now time.Time) nodeTrustStatus {
	warning, critical := rt.nodeCerts.thresholds()
	switch cs, _ := alerting.ClassifyNodeCert(*n, warning, critical, now); cs {
	case alerting.NodeCertRevoked:
		return nodeTrustStatus{State: trustRevoked}
	case alerting.NodeCertExpired:
		return nodeTrustStatus{State: trustExpired}
	}
	acc := newestConnect(ctx, ts, n.ID, store.SecurityOutcomeAccepted)
	if acc != nil && n.Status == store.NodeStatusOnline && n.AcceptsCert(acc.Detail.Fingerprint, now) {
		return nodeTrustStatus{State: trustVerified}
	}
	if rej := newestConnect(ctx, ts, n.ID, store.SecurityOutcomeRejected); rej != nil && now.Sub(rej.LastAt) <= trustWindow {
		return nodeTrustStatus{State: trustRejected, Reason: rej.Reason}
	}
	if acc != nil {
		return nodeTrustStatus{State: trustIdle}
	}
	return nodeTrustStatus{State: trustNoActivity}
}

func newestConnect(ctx context.Context, ts nodeTrustStore, nodeID, outcome string) *store.NodeSecurityEvent {
	evs, err := ts.ListNodeSecurityEvents(ctx, nodeID, store.NodeSecurityFilter{Outcome: outcome, Limit: 20})
	if err != nil {
		return nil
	}
	for i := range evs {
		if evs[i].Kind == store.SecurityKindConnect {
			return &evs[i]
		}
	}
	return nil
}

// handleListNodeSecurityEvents handles GET /api/v1/nodes/{id}/security/events.
func (rt *Router) handleListNodeSecurityEvents(w http.ResponseWriter, r *http.Request) {
	n, ts, ok := rt.trustNode(w, r)
	if !ok {
		return
	}
	f := store.NodeSecurityFilter{Limit: defaultSecurityEventsLimit, Outcome: r.URL.Query().Get("outcome")}
	switch f.Outcome {
	case "", store.SecurityOutcomeAccepted, store.SecurityOutcomeRejected, store.SecurityOutcomeWarning, store.SecurityOutcomeFailed, store.SecurityOutcomeInfo:
	default:
		writeError(w, http.StatusBadRequest, "outcome must be accepted, rejected, warning, failed or info")
		return
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 || v > maxSecurityEventsLimit {
			writeError(w, http.StatusBadRequest, "limit must be an integer between 1 and "+strconv.Itoa(maxSecurityEventsLimit))
			return
		}
		f.Limit = v
	}
	evs, err := ts.ListNodeSecurityEvents(r.Context(), n.ID, f)
	if err != nil {
		rt.internalError(w, "api: list node security events failed", err, slog.String("node_id", n.ID))
		return
	}
	out := make([]nodeSecurityEventResource, 0, len(evs))
	for _, e := range evs {
		out = append(out, nodeSecurityEventResource{
			ID: e.ID, Kind: e.Kind, Outcome: e.Outcome, Reason: e.Reason, SourceIP: e.SourceIP, Count: e.Count, FirstAt: e.FirstAt, LastAt: e.LastAt,
			TokenID: e.Detail.TokenID, Subject: e.Detail.Subject, Message: e.Detail.Message, Serial: e.Detail.Serial,
			Fingerprint: e.Detail.Fingerprint, NotBefore: e.Detail.NotBefore, NotAfter: e.Detail.NotAfter, Actor: e.Detail.Actor,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRenewNodeCert handles POST /api/v1/nodes/{id}/cert/renew: asks the
// connected agent to renew its certificate now.
func (rt *Router) handleRenewNodeCert(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := rt.nodes.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: renew node cert: look up node failed", err, slog.String("node_id", id))
		return
	}
	if rt.isLocalNodeRecord(n) {
		writeError(w, http.StatusBadRequest, errLocalNodeNoCert)
		return
	}
	if n.CertRevokedAt != nil {
		writeError(w, http.StatusConflict, "the node's certificate is revoked: re-enroll it instead")
		return
	}
	if rt.nodeCerts.trust == nil {
		writeError(w, http.StatusNotImplemented, "certificate renewal on demand is not available on this control plane")
		return
	}
	if err := rt.nodeCerts.trust.RequestRenew(id); errors.Is(err, agent.ErrNodeNotConnected) {
		writeError(w, http.StatusConflict, "the node is not connected: it renews by itself when it reconnects")
		return
	} else if err != nil {
		rt.logger.Error("api: renew node cert: request failed", slog.String("node_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "could not reach the node's agent")
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	rt.nodeCerts.trust.RecordSecurityEvent(store.NodeSecurityEvent{
		NodeID: id, Kind: store.SecurityKindRenewRequest, Outcome: store.SecurityOutcomeInfo, Detail: store.NodeSecurityDetail{Actor: actor},
	})
	rt.logger.Info("api: node certificate renewal requested", slog.String("node_id", id))
	writeJSON(w, http.StatusAccepted, map[string]bool{"requested": true})
}

// recordOperatorEvent logs an operator action against a node's trust.
func (rt *Router) recordOperatorEvent(r *http.Request, nodeID, kind, reason string) {
	if rt.nodeCerts.trust == nil {
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	rt.nodeCerts.trust.RecordSecurityEvent(store.NodeSecurityEvent{
		NodeID: nodeID, Kind: kind, Outcome: store.SecurityOutcomeInfo, Reason: reason, Detail: store.NodeSecurityDetail{Actor: actor},
	})
}
