package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	// DefaultCertDrillTimeout is how long a rotation drill waits for the new certificate.
	DefaultCertDrillTimeout = 30 * time.Second
	defaultCertDrillPoll    = 500 * time.Millisecond

	drillStepPreflight = "preflight"
	drillStepRequest   = "request"
	drillStepIssued    = "new_certificate"
	drillStepOnline    = "still_online"

	drillPass = "pass"
	drillFail = "fail"
	drillSkip = "skipped"
)

// WithNodeCertDrillTimeout sets how long a certificate rotation drill waits for the new certificate.
func WithNodeCertDrillTimeout(d time.Duration) Option {
	return func(rt *Router) { rt.nodeCerts.drillTimeout = d }
}

type nodeCertDrillStep struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type nodeCertDrillCert struct {
	Serial     string     `json:"serial,omitempty"`
	Generation int        `json:"generation"`
	NotAfter   *time.Time `json:"not_after,omitempty"`
}

type nodeCertDrillResult struct {
	OK     bool                `json:"ok"`
	Steps  []nodeCertDrillStep `json:"steps"`
	Before nodeCertDrillCert   `json:"before"`
	After  nodeCertDrillCert   `json:"after"`
}

func drillCertOf(n *store.Node) nodeCertDrillCert {
	return nodeCertDrillCert{Serial: n.CertSerial, Generation: n.CertGeneration, NotAfter: n.CertNotAfter}
}

func (d *nodeCertDrillResult) add(id, status, detail string) {
	d.Steps = append(d.Steps, nodeCertDrillStep{ID: id, Status: status, Detail: detail})
}

func (d *nodeCertDrillResult) skipRest(ids ...string) {
	for _, id := range ids {
		d.add(id, drillSkip, "not run because an earlier step failed")
	}
}

// handleNodeCertDrill handles POST /api/v1/nodes/{id}/cert/drill. Safe order: check the node
// can renew, ask it to renew, wait for a new certificate, confirm it is still online. The old
// certificate is never revoked, so a failed drill leaves the node as it was.
func (rt *Router) handleNodeCertDrill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	n, err := rt.nodes.GetNode(r.Context(), id)
	if errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: cert drill: look up node failed", err)
		return
	}
	if rt.isLocalNodeRecord(n) {
		writeError(w, http.StatusBadRequest, errLocalNodeNoCert)
		return
	}
	res := nodeCertDrillResult{Steps: []nodeCertDrillStep{}, Before: drillCertOf(n)}
	res.After = res.Before
	rest := []string{drillStepRequest, drillStepIssued, drillStepOnline}

	if msg := rt.drillPreflight(n); msg != "" {
		res.add(drillStepPreflight, drillFail, msg)
		res.skipRest(rest...)
		writeJSON(w, http.StatusOK, res)
		return
	}
	res.add(drillStepPreflight, drillPass, "the node is online and holds a valid certificate")

	if err := rt.nodeCerts.trust.RequestRenew(id); err != nil {
		msg := "could not reach the node's agent"
		if errors.Is(err, agent.ErrNodeNotConnected) {
			msg = "the node is not connected"
		}
		rt.logger.Warn("api: cert drill: renew request failed", slog.String("node_id", id), slog.String("error", err.Error()))
		res.add(drillStepRequest, drillFail, msg)
		res.skipRest(rest[1:]...)
		writeJSON(w, http.StatusOK, res)
		return
	}
	rt.recordOperatorEvent(r, id, store.SecurityKindRenewRequest, "certificate rotation drill")
	res.add(drillStepRequest, drillPass, "the agent was asked to renew")

	after, ok := rt.waitForNewCert(r.Context(), id, n)
	if after != nil {
		res.After = drillCertOf(after)
	}
	if !ok {
		res.add(drillStepIssued, drillFail, "no new certificate arrived in time; the old one is still valid and was not touched")
		res.skipRest(drillStepOnline)
		writeJSON(w, http.StatusOK, res)
		return
	}
	res.add(drillStepIssued, drillPass, "a new certificate was issued and stored")

	final, err := rt.nodes.GetNode(r.Context(), id)
	switch {
	case err != nil:
		res.add(drillStepOnline, drillFail, "could not read the node after renewal")
	case final.Status != store.NodeStatusOnline && final.Status != store.NodeStatusCordoned:
		res.add(drillStepOnline, drillFail, "the node went "+string(final.Status)+" after renewal")
	default:
		res.add(drillStepOnline, drillPass, "the node is still connected with the new certificate")
		res.OK = true
	}
	rt.logger.Info("api: node certificate rotation drill finished", slog.String("node_id", id), slog.Bool("ok", res.OK))
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) drillPreflight(n *store.Node) string {
	switch {
	case rt.nodeCerts.trust == nil:
		return "certificate renewal on demand is not available on this control plane"
	case n.CertRevokedAt != nil:
		return "the node's certificate is revoked: re-enroll it instead"
	case n.Status == store.NodeStatusOffline || n.Status == store.NodeStatusPending:
		return "the node is not connected: it renews by itself when it reconnects"
	case n.CertNotAfter == nil || !n.CertNotAfter.After(time.Now()):
		return "the node holds no valid certificate: re-enroll it instead"
	}
	return ""
}

func (rt *Router) waitForNewCert(ctx context.Context, id string, before *store.Node) (*store.Node, bool) {
	timeout, poll := rt.nodeCerts.drillTimeout, rt.nodeCerts.drillPoll
	if timeout <= 0 {
		timeout = DefaultCertDrillTimeout
	}
	if poll <= 0 {
		poll = defaultCertDrillPoll
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tick := time.NewTicker(poll)
	defer tick.Stop()
	var last *store.Node
	for {
		n, err := rt.nodes.GetNode(ctx, id)
		if err == nil {
			last = n
			if n.CertGeneration > before.CertGeneration || (n.CertSerial != "" && n.CertSerial != before.CertSerial) {
				return n, true
			}
		}
		select {
		case <-ctx.Done():
			return last, false
		case <-tick.C:
		}
	}
}
