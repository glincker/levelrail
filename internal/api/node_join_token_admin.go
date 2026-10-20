package api

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// joinTokenAdmin is the optional store surface behind listing and revoking
// outstanding join tokens; *store.DB satisfies it.
type joinTokenAdmin interface {
	ListPendingNodeJoinTokens(ctx context.Context, now time.Time) ([]store.NodeJoinToken, error)
	RevokePendingNodeJoinTokens(ctx context.Context, id string, now time.Time) (int64, error)
}

type pendingJoinTokenResource struct {
	ID        string    `json:"id"`
	Purpose   string    `json:"purpose"`
	NodeID    string    `json:"node_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type revokeJoinTokensResponse struct {
	Revoked int64 `json:"revoked"`
}

// handleListPendingNodeJoinTokens handles GET /api/v1/nodes/join-tokens.
func (rt *Router) handleListPendingNodeJoinTokens(w http.ResponseWriter, r *http.Request) {
	adm, ok := rt.nodes.(joinTokenAdmin)
	if !ok {
		writeError(w, http.StatusNotImplemented, "join token management is not available")
		return
	}
	toks, err := adm.ListPendingNodeJoinTokens(r.Context(), time.Now())
	if err != nil {
		rt.internalError(w, "list pending join tokens", err)
		return
	}
	out := make([]pendingJoinTokenResource, 0, len(toks))
	for _, t := range toks {
		out = append(out, pendingJoinTokenResource{ID: t.ID, Purpose: t.Purpose, NodeID: t.NodeID, CreatedAt: t.CreatedAt, ExpiresAt: t.ExpiresAt})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeNodeJoinTokens handles DELETE /api/v1/nodes/join-tokens, revoking every unused token.
func (rt *Router) handleRevokeNodeJoinTokens(w http.ResponseWriter, r *http.Request) {
	rt.revokeJoinTokens(w, r, "")
}

// handleRevokeNodeJoinToken handles DELETE /api/v1/nodes/join-tokens/{id}.
func (rt *Router) handleRevokeNodeJoinToken(w http.ResponseWriter, r *http.Request) {
	rt.revokeJoinTokens(w, r, r.PathValue("id"))
}

func (rt *Router) revokeJoinTokens(w http.ResponseWriter, r *http.Request, id string) {
	adm, ok := rt.nodes.(joinTokenAdmin)
	if !ok {
		writeError(w, http.StatusNotImplemented, "join token management is not available")
		return
	}
	n, err := adm.RevokePendingNodeJoinTokens(r.Context(), id, time.Now())
	if err != nil {
		rt.internalError(w, "revoke join tokens", err)
		return
	}
	if id != "" && n == 0 {
		writeError(w, http.StatusNotFound, "no pending join token with that id")
		return
	}
	rt.logger.Warn("api: node join tokens revoked", slog.Int64("count", n), slog.String("token_id", id))
	writeJSON(w, http.StatusOK, revokeJoinTokensResponse{Revoked: n})
}
