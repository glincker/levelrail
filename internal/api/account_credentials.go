package api

import (
	"log/slog"
	"net/http"
	"time"
)

type myCredentialsResponse struct {
	Sessions []sessionResource `json:"sessions"`
	Tokens   []tokenResource   `json:"tokens"`
}

// handleMyCredentials handles GET /api/v1/auth/my-credentials: the caller's own
// live sessions and active tokens, read-only so an assistant can review them.
// A token secret is never part of a token resource.
func (rt *Router) handleMyCredentials(w http.ResponseWriter, r *http.Request) {
	userID, viaSession, ok := rt.accountCaller(w, r, AbilityReadSensitive)
	if !ok {
		return
	}
	out := myCredentialsResponse{Sessions: []sessionResource{}, Tokens: []tokenResource{}}
	current := ""
	if viaSession {
		current = currentSessionToken(r)
	}
	now := time.Now()
	views, err := rt.libSessions.ListUserSessions(r.Context(), userID, current)
	if err != nil {
		rt.internalError(w, "api: my credentials: sessions failed", err, slog.String("user_id", userID))
		return
	}
	for _, v := range views {
		out.Sessions = append(out.Sessions, sessionResource{ID: v.ID, Browser: browserLabel(v.UserAgent), IP: safeSignInIP(v.IP), Network: ipNetworkPrefix(v.IP),
			CreatedAt: v.CreatedAt, LastSeenAt: v.LastSeenAt, ExpiresAt: v.ExpiresAt, Current: v.Current,
			CanApprove: rt.codeLogin.approverFreshness <= 0 || now.Sub(v.LastSeenAt) <= rt.codeLogin.approverFreshness})
	}
	tokens, err := rt.libraryListTokens(r.Context(), userID, false)
	if err != nil {
		rt.internalError(w, "api: my credentials: tokens failed", err, slog.String("user_id", userID))
		return
	}
	for _, t := range tokens {
		if t.RevokedAt == nil {
			out.Tokens = append(out.Tokens, t)
		}
	}
	writeJSON(w, http.StatusOK, out)
}
