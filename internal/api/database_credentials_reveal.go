package api

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	dbRevealStepUpApp   = "database-credentials"
	dbRevealAuditAction = "database.credentials.reveal"
	envDBRevealPerMin   = "APP_DB_REVEAL_PER_MINUTE"
	defaultDBRevealRate = 5
)

type databaseRevealRequest struct {
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

type databaseRevealResponse struct {
	Database string `json:"database"`
	Engine   string `json:"engine"`
	Password string `json:"password"`
}

func newDBRevealLimiter() *apiRateLimiter {
	n := defaultDBRevealRate
	if v, err := strconv.Atoi(os.Getenv(envDBRevealPerMin)); err == nil && v > 0 {
		n = v
	}
	return newAPIRateLimiter(n)
}

// handleRevealDatabaseCredentials handles POST /api/v1/databases/{name}/credentials/reveal.
// Owner-only (root plus a fresh step-up), rate limited per user, audited, and
// the password is never logged or cached.
func (rt *Router) handleRevealDatabaseCredentials(w http.ResponseWriter, r *http.Request) {
	if rt.secrets == nil {
		writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane")
		return
	}
	var req databaseRevealRequest
	if err := decodeJSONBodyLimit(r, &req, stepUpBodyLimit); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	name := r.PathValue("name")
	userID, ok := rt.stepUpOwner(w, r, req.StepUp, dbRevealStepUpApp+":"+name, "reveal a database password", "revealing credentials")
	if !ok {
		return
	}
	if allowed, retry := rt.dbRevealLimit.allow(userID); !allowed {
		writeRateLimited(w, retry)
		return
	}
	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	key, hasKey := database.PasswordSecretKey(d.Engine)
	if !hasKey {
		writeError(w, http.StatusBadRequest, "this engine has no generated password")
		return
	}
	pw, err := rt.secrets.Resolve(r.Context(), name, key)
	if err != nil || pw == "" {
		writeError(w, http.StatusNotFound, "no password is stored for this database")
		return
	}
	rt.recordDBRevealAudit(r, name, userID)
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, databaseRevealResponse{Database: name, Engine: d.Engine, Password: pw})
}

func (rt *Router) recordDBRevealAudit(r *http.Request, name, userID string) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: database reveal audit id failed", slog.String("database", name))
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	if actor == "" {
		actor = userID
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "user", ActorName: actor, Ability: AbilityRoot, Method: dbRevealAuditAction,
		Path: "/api/v1/databases/" + name, StatusCode: http.StatusOK,
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "api",
	}
	if err := rt.auditLog.SaveAuditEntry(r.Context(), entry); err != nil {
		rt.logger.Warn("api: database reveal audit save failed", slog.String("database", name), slog.String("error", err.Error()))
	}
}
