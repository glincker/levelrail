package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/dbaccess"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

type databaseTLSModeStore interface {
	GetDatabaseTLSMode(ctx context.Context, database string) (string, error)
	SetDatabaseTLSMode(ctx context.Context, database, mode string) error
}

type databaseTLSValueDeleter interface {
	DeleteValue(ctx context.Context, serviceName, envKey string) error
}

type databaseTLSToggleRequest struct {
	Enabled bool `json:"enabled"`
}

type databaseTLSToggleResponse struct {
	Database string `json:"database"`
	Mode     string `json:"mode"`
	Enabled  bool   `json:"enabled"`
}

func (rt *Router) databaseTLSModeStore() (databaseTLSModeStore, bool) {
	m, ok := rt.dbAccess.(databaseTLSModeStore)
	return m, ok
}

// databaseTLSMode returns the stored mode, or default when unreadable so a read never blocks a page.
func (rt *Router) databaseTLSMode(ctx context.Context, name string) string {
	m, ok := rt.databaseTLSModeStore()
	if !ok {
		return store.DatabaseTLSModeDefault
	}
	mode, err := m.GetDatabaseTLSMode(ctx, name)
	if err != nil {
		rt.logger.Warn("api: database tls mode unreadable", slog.String("name", name), slog.String("error", err.Error()))
		return store.DatabaseTLSModeDefault
	}
	return mode
}

// handleSetDatabaseTLSEnabled handles PUT /api/v1/databases/{name}/tls: switches server TLS for MySQL, MariaDB and Redis.
func (rt *Router) handleSetDatabaseTLSEnabled(w http.ResponseWriter, r *http.Request) {
	var req databaseTLSToggleRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, errBadBody)
		return
	}
	modes, ok := rt.databaseTLSModeStore()
	if !ok || rt.dbAccess == nil {
		writeError(w, http.StatusNotImplemented, errDatabaseAccessOff)
		return
	}
	d, ok := rt.dbAccessDatabase(w, r)
	if !ok {
		return
	}
	if !store.DatabaseTLSToggleable(d.Engine) {
		writeError(w, http.StatusBadRequest, "TLS can be switched for MySQL, MariaDB and Redis databases only")
		return
	}
	mode := store.DatabaseTLSModeOff
	if req.Enabled {
		mode = store.DatabaseTLSModeOn
	}
	if !req.Enabled {
		del, canDelete := rt.secrets.(databaseTLSValueDeleter)
		if !canDelete {
			writeError(w, http.StatusNotImplemented, "this control plane has no master key configured, so TLS cannot be switched")
			return
		}
		for _, key := range []string{database.TLSCertEnvKey, database.TLSKeyEnvKey} {
			if err := del.DeleteValue(r.Context(), d.Name, key); err != nil {
				rt.internalError(w, "api: database tls: remove material failed", err, slog.String("name", d.Name))
				return
			}
		}
	}
	if err := modes.SetDatabaseTLSMode(r.Context(), d.Name, mode); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		rt.internalError(w, "api: database tls: save mode failed", err, slog.String("name", d.Name))
		return
	}
	rt.auditDatabaseAccess(r, dbaccess.ActionTLSToggle, d.Name, mode, http.StatusOK)
	rt.logger.Info("api: database tls switched", slog.String("database", d.Name), slog.String("mode", mode))
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, databaseTLSToggleResponse{Database: d.Name, Mode: mode, Enabled: req.Enabled})
}
