package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// databasePublicAccessResource is PUT/DELETE
// /api/v1/databases/{name}/public-access's response body: just the three
// fields that endpoint actually changes, the same "deliberately its own
// small type, not a reuse of databaseResource" reasoning
// backupScheduleResource's own doc comment gives (backups.go).
type databasePublicAccessResource struct {
	DatabaseName       string `json:"database_name"`
	PubliclyAccessible bool   `json:"publicly_accessible"`
	PublicPort         int    `json:"public_port,omitempty"`
}

// setDatabasePublicAccessRequest is handleSetDatabasePublicAccess's
// request body. Port is optional: 0 (the zero value, indistinguishable
// from "omitted" in this handler, the same convention
// setBackupScheduleRequest's own Retain field already uses) means "auto-
// assign the next free port", store.SetDatabasePublicAccess's own
// default when requestedPort is 0.
type setDatabasePublicAccessRequest struct {
	Port int `json:"port,omitempty"`
}

// reservedControlPlanePorts are ports this control plane's own listeners
// already use (cmd/levelrail's defaultHTTPAddr ":8080", defaultAgentAddr
// ":9443") plus the two Caddy ingress binds every embedded install
// reserves. An explicit public-access port request landing on one of
// these would silently fail to bind or, worse, collide with a listener
// already using it, so this handler rejects the request outright rather
// than letting Docker's own bind error surface later as an opaque
// reconcile failure.
var reservedControlPlanePorts = map[int]bool{
	80:   true,
	443:  true,
	8080: true,
	9443: true,
}

// handleSetDatabasePublicAccess handles
// PUT /api/v1/databases/{name}/public-access: the missing link that lets
// an operator connect a database GUI tool (pgAdmin, TablePlus,
// RedisInsight) to a managed database from their own machine, not just
// from inside the Docker network. store.SetDatabasePublicAccess does the
// real work (port allocation or validation, persisted atomically); this
// handler is request validation plus the not-found/conflict mapping.
//
// A requested port outside the ephemeral-safe range this control plane
// itself never binds (1024-65535, reservedControlPlanePorts excluded) is
// rejected as a 400 rather than passed through to Docker, the same
// "validate everything this handler can before ever writing state"
// discipline handleSetBackupSchedule's own doc comment describes for its
// cron expression.
func (rt *Router) handleSetDatabasePublicAccess(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: set database public access: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var req setDatabasePublicAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Port != 0 {
		if req.Port < 1024 || req.Port > 65535 {
			writeError(w, http.StatusBadRequest, "port must be between 1024 and 65535")
			return
		}
		if reservedControlPlanePorts[req.Port] {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("port %d is reserved by the control plane itself", req.Port))
			return
		}
	}

	port, err := rt.databases.SetDatabasePublicAccess(r.Context(), name, true, req.Port)
	switch {
	case errors.Is(err, store.ErrDatabaseNotFound):
		writeError(w, http.StatusNotFound, "database not found")
		return
	case errors.Is(err, store.ErrPublicPortInUse):
		writeError(w, http.StatusConflict, "that port is already assigned to another database")
		return
	case errors.Is(err, store.ErrPublicPortRangeExhausted):
		writeError(w, http.StatusConflict, "no free port available for auto-assignment; request a specific port instead")
		return
	case err != nil:
		rt.logger.Error("api: set database public access failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	writeJSON(w, http.StatusOK, databasePublicAccessResource{
		DatabaseName:       name,
		PubliclyAccessible: true,
		PublicPort:         port,
	})
}

// handleClearDatabasePublicAccess handles
// DELETE /api/v1/databases/{name}/public-access: the reverse of
// handleSetDatabasePublicAccess, returning name to its default
// internal-network-only state. The next reconcile pass sees
// PubliclyAccessible flip to false and replaces the running container
// without the host port binding, the same "replace, not restart"
// consequence this feature adds a case for in
// internal/reconcile/database's own controller.
func (rt *Router) handleClearDatabasePublicAccess(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.databases.GetDesiredDatabase(r.Context(), name); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear database public access: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if _, err := rt.databases.SetDatabasePublicAccess(r.Context(), name, false, 0); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.logger.Error("api: clear database public access failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
