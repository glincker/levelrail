package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

type auditChainVerifier interface {
	VerifyAuditChain(ctx context.Context) (store.AuditChainReport, error)
}

// handleVerifyAuditLog handles GET /api/v1/audit-log/verify: walks the audit
// hash chain and reports edited, removed or truncated rows.
func (rt *Router) handleVerifyAuditLog(w http.ResponseWriter, r *http.Request) {
	v, ok := rt.auditLog.(auditChainVerifier)
	if !ok {
		writeError(w, http.StatusNotImplemented, "audit log verification is not available on this store")
		return
	}
	report, err := v.VerifyAuditChain(r.Context())
	if err != nil {
		rt.internalError(w, "api: verify audit chain failed", err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
