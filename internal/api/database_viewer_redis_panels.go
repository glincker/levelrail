package api

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const redisClientsAuditAction = "database.redis.clients"

// handleGetRedisInfo handles GET /api/v1/databases/{name}/redis/info: fixed
// INFO sections, allow listed.
func (rt *Router) handleGetRedisInfo(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.viewerTarget(w, r, true)
	if !ok {
		return
	}
	info, err := target.Info(r.Context())
	if err != nil {
		rt.writeViewerError(w, "api: redis info failed", desired.Name, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleGetRedisSlowlog handles GET /api/v1/databases/{name}/redis/slowlog.
// Arguments never leave the server, only the command name and sizes.
func (rt *Router) handleGetRedisSlowlog(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.viewerTarget(w, r, true)
	if !ok {
		return
	}
	count := 0
	if raw := r.URL.Query().Get("count"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			writeError(w, http.StatusBadRequest, "count must be a positive integer")
			return
		}
		count = n
	}
	log, err := target.Slowlog(r.Context(), count)
	if err != nil {
		rt.writeViewerError(w, "api: redis slowlog failed", desired.Name, err)
		return
	}
	writeJSON(w, http.StatusOK, log)
}

// handleGetRedisClients handles GET /api/v1/databases/{name}/redis/clients.
// Addresses are returned, so every read is audited.
func (rt *Router) handleGetRedisClients(w http.ResponseWriter, r *http.Request) {
	target, desired, ok := rt.viewerTarget(w, r, true)
	if !ok {
		return
	}
	clients, err := target.Clients(r.Context())
	if err != nil {
		rt.writeViewerError(w, "api: redis clients failed", desired.Name, err)
		return
	}
	rt.recordRedisClientsAudit(r, desired.Name)
	writeJSON(w, http.StatusOK, clients)
}

func (rt *Router) recordRedisClientsAudit(r *http.Request, name string) {
	if rt.auditLog == nil {
		return
	}
	pt, pid, _, err := rt.callerPrincipal(r)
	if err != nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: redis clients audit id failed", slog.String("database", name))
		return
	}
	actorType := "session"
	if pt == store.PrincipalTypeToken {
		actorType = "token"
	}
	entry := store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: pid,
		ActorName: rt.auditActorName(r.Context(), actorType, pid, pid),
		Ability:   AbilityReadSensitive, Method: redisClientsAuditAction,
		Path:       r.URL.Path,
		StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt:  store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: redis clients audit save failed", slog.String("database", name), slog.String("error", err.Error()))
	}
}
