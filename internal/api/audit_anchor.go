package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/auditanchor"
	"github.com/GLINCKER/levelrail/internal/store"
)

// anchorMaxBody bounds an anchor check body; an export line is about 200 bytes.
const anchorMaxBody = 8 << 20

type auditAnchorStore interface {
	LatestAuditAnchor(ctx context.Context) (store.AuditAnchor, bool, error)
	CheckAuditAnchors(ctx context.Context, anchors []store.AuditAnchor) (store.AuditAnchorReport, error)
}

// handleGetAuditAnchor handles GET /api/v1/audit-log/anchor: the signed chain
// head an operator can copy off the box. It holds no row content.
func (rt *Router) handleGetAuditAnchor(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.auditLog.(auditAnchorStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "audit anchors are not available on this store")
		return
	}
	a, found, err := s.LatestAuditAnchor(r.Context())
	if err != nil {
		rt.internalError(w, "api: read audit anchor failed", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "the audit log has no chained entries yet")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

type auditAnchorCheckRequest struct {
	Anchors []store.AuditAnchor `json:"anchors"`
}

// handleCheckAuditAnchors handles POST /api/v1/audit-log/anchor-check: confirms
// the live chain still contains every anchored head the caller supplies.
func (rt *Router) handleCheckAuditAnchors(w http.ResponseWriter, r *http.Request) {
	s, ok := rt.auditLog.(auditAnchorStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "audit anchors are not available on this store")
		return
	}
	var req auditAnchorCheckRequest
	if err := decodeJSONBodyLimit(r, &req, anchorMaxBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid anchor list")
		return
	}
	if len(req.Anchors) == 0 {
		writeError(w, http.StatusBadRequest, "anchors is required")
		return
	}
	report, err := s.CheckAuditAnchors(r.Context(), req.Anchors)
	if err != nil {
		rt.internalError(w, "api: check audit anchors failed", err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// SetAnchorStatus wires the webhook exporter's status snapshot.
func (rt *Router) SetAnchorStatus(f func(context.Context) auditanchor.Status) { rt.anchorStatus = f }

// handleGetAuditAnchorStatus handles GET /api/v1/audit-log/anchor-status: the
// last exported sequence and last error of the webhook export.
func (rt *Router) handleGetAuditAnchorStatus(w http.ResponseWriter, r *http.Request) {
	if rt.anchorStatus == nil {
		writeJSON(w, http.StatusOK, auditanchor.Status{})
		return
	}
	writeJSON(w, http.StatusOK, rt.anchorStatus(r.Context()))
}
