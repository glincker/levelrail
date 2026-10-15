package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// auditChainVerifier is the optional store surface behind the verify route.
type auditChainVerifier interface {
	VerifyAuditChain(ctx context.Context) (store.AuditChainReport, error)
}

type auditChainBreakResource struct {
	Seq    int64  `json:"seq"`
	ID     string `json:"id,omitempty"`
	Reason string `json:"reason"`
}

// auditChainResource is GET /api/v1/audit-log/verify's wire shape.
type auditChainResource struct {
	OK          bool                     `json:"ok"`
	Chained     int64                    `json:"chained"`
	Unchained   int64                    `json:"unchained"`
	HeadSeq     int64                    `json:"head_seq"`
	HeadHash    string                   `json:"head_hash"`
	FirstBroken *auditChainBreakResource `json:"first_broken,omitempty"`
}

// handleVerifyAuditChain walks the hash chain and reports the first broken
// link. Rows written before the chain existed are counted, not checked.
func (rt *Router) handleVerifyAuditChain(w http.ResponseWriter, r *http.Request) {
	v, ok := rt.auditLog.(auditChainVerifier)
	if !ok {
		writeError(w, http.StatusNotImplemented, "audit chain verification is not available")
		return
	}
	rep, err := v.VerifyAuditChain(r.Context())
	if err != nil {
		rt.internalError(w, "api: verify audit chain failed", err)
		return
	}
	out := auditChainResource{
		OK: rep.FirstBroken == nil, Chained: rep.Chained, Unchained: rep.Unchained,
		HeadSeq: rep.HeadSeq, HeadHash: rep.HeadHash,
	}
	if rep.FirstBroken != nil {
		out.FirstBroken = &auditChainBreakResource{Seq: rep.FirstBroken.Seq, ID: rep.FirstBroken.ID, Reason: rep.FirstBroken.Reason}
	}
	writeJSON(w, http.StatusOK, out)
}
