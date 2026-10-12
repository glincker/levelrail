package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultGateEventWindow = 24 * time.Hour
	defaultGateEventLimit  = 500
	maxGateEventLimit      = 5000
)

type authGateEventResource struct {
	At        time.Time `json:"at"`
	UserID    string    `json:"user_id"`
	UserEmail string    `json:"user_email"`
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason"`
	IP        string    `json:"ip"`
	Count     int       `json:"count"`
}

type authGateEventsResponse struct {
	Domain string                  `json:"domain"`
	Since  time.Time               `json:"since"`
	Events []authGateEventResource `json:"events"`
}

type gateSessionsRevokedResponse struct {
	Domain  string `json:"domain"`
	Revoked int64  `json:"revoked"`
}

// handleListAuthGateEvents handles GET .../auth-gate/events?since=24h&limit=500,
// the built-in provider's access log, newest first.
func (rt *Router) handleListAuthGateEvents(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	if rt.gate == nil {
		writeError(w, http.StatusNotImplemented, "the built-in sign-in provider is not available on this server")
		return
	}
	window := defaultGateEventWindow
	if raw := r.URL.Query().Get("since"); raw != "" {
		d, err := parseAuthGateDuration(raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "since: "+err.Error())
			return
		}
		window = d
	}
	limit := defaultGateEventLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxGateEventLimit {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and "+strconv.Itoa(maxGateEventLimit))
			return
		}
		limit = n
	}
	rt.flushGateEvents(r.Context())
	since := time.Now().UTC().Add(-window)
	events, err := rt.gate.ListAuthGateEvents(r.Context(), domain, since, limit)
	if err != nil {
		rt.internalError(w, "api: list auth gate events failed", err, slog.String("domain", domain))
		return
	}
	out := authGateEventsResponse{Domain: domain, Since: since, Events: make([]authGateEventResource, 0, len(events))}
	for _, e := range events {
		out.Events = append(out.Events, authGateEventResource{At: e.At, UserID: e.UserID, UserEmail: e.UserEmail, Decision: e.Decision, Reason: e.Reason, IP: e.IP, Count: e.Count})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRevokeAuthGateSessions handles DELETE .../auth-gate/sessions: every
// visitor signed in to domain through the built-in provider signs in again.
func (rt *Router) handleRevokeAuthGateSessions(w http.ResponseWriter, r *http.Request) {
	domain, ok := rt.requireOwnedDomain(w, r)
	if !ok {
		return
	}
	if rt.gate == nil {
		writeError(w, http.StatusNotImplemented, "the built-in sign-in provider is not available on this server")
		return
	}
	n, err := rt.gate.RevokeGateSessions(r.Context(), domain)
	if err != nil {
		rt.internalError(w, "api: revoke auth gate sessions failed", err, slog.String("domain", domain))
		return
	}
	e := store.GateEvent{Domain: domain, Decision: store.GateDecisionSessionRevoked, Reason: "revoked_by_admin", IP: clientIP(r)}
	if actorType, actorID, _, ok := rt.currentActor(r); ok && actorType == store.PrincipalTypeUser {
		e.UserID = actorID
		if u, err := rt.gate.GetUserByID(r.Context(), actorID); err == nil {
			e.UserEmail = u.Email
		}
	}
	rt.recordGateEvent(r.Context(), e)
	writeJSON(w, http.StatusOK, gateSessionsRevokedResponse{Domain: domain, Revoked: n})
}
