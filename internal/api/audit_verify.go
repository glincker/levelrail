package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/store"
)

type auditChainVerifier interface {
	VerifyAuditChain(ctx context.Context) (store.AuditChainReport, error)
}

type auditChainRangeVerifier interface {
	VerifyAuditChainRange(ctx context.Context, from, to int64) (store.AuditChainReport, error)
}

// handleVerifyAuditLog handles GET /api/v1/audit-log/verify: walks the audit
// hash chain and reports edited, removed or truncated rows. ?from and ?to
// (chain sequence numbers) limit what is reported to that range.
func (rt *Router) handleVerifyAuditLog(w http.ResponseWriter, r *http.Request) {
	var bounds [2]int64
	for i, name := range []string{"from", "to"} {
		raw := r.URL.Query().Get(name)
		if raw == "" {
			continue
		}
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, name+" must be a non-negative chain sequence number")
			return
		}
		bounds[i] = n
	}
	if bounds[1] != 0 && bounds[0] > bounds[1] {
		writeError(w, http.StatusBadRequest, "from must not be greater than to")
		return
	}
	var (
		report store.AuditChainReport
		err    error
	)
	if rv, ok := rt.auditLog.(auditChainRangeVerifier); ok {
		report, err = rv.VerifyAuditChainRange(r.Context(), bounds[0], bounds[1])
	} else if v, ok := rt.auditLog.(auditChainVerifier); ok {
		report, err = v.VerifyAuditChain(r.Context())
	} else {
		writeError(w, http.StatusNotImplemented, "audit log verification is not available on this store")
		return
	}
	if err != nil {
		rt.internalError(w, "api: verify audit chain failed", err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
