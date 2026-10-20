package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/dbcatalogbuild"
	"github.com/GLINCKER/levelrail/internal/dbupgrade"
)

// DatabaseVersionCatalog is the remote-refreshed version catalog
// (internal/dbupgrade.RemoteCatalog).
type DatabaseVersionCatalog interface {
	Engines() []string
	Versions(engine string) (dbupgrade.VersionsView, error)
	LiveRefresh(ctx context.Context, engine string) (dbupgrade.VersionsView, bool, error)
	Status() dbupgrade.Status
	Refresh(ctx context.Context) error
	SetRemoteEnabled(enabled bool) error
}

// SetDatabaseVersionCatalog enables the database version catalog routes.
func (rt *Router) SetDatabaseVersionCatalog(c DatabaseVersionCatalog) { rt.dbVersions = c }

func (rt *Router) versionsOr501(w http.ResponseWriter) bool {
	if rt.dbVersions == nil {
		writeError(w, http.StatusNotImplemented, "the database version catalog is not configured on this control plane")
		return false
	}
	return true
}

func writeVersionsError(w http.ResponseWriter, err error) {
	var rl *dbcatalogbuild.RateLimitedError
	switch {
	case errors.Is(err, dbupgrade.ErrUnknownEngine), errors.Is(err, dbupgrade.ErrUnsupportedEngine):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, dbupgrade.ErrRemoteDisabled):
		writeError(w, http.StatusConflict, err.Error())
	case errors.As(err, &rl):
		w.Header().Set("Retry-After", strconv.Itoa(int(rl.RetryAfter.Seconds())+1))
		writeError(w, http.StatusTooManyRequests, "the upstream registry is rate limiting this control plane, try again later")
	default:
		writeError(w, http.StatusBadGateway, "the live version lookup failed: "+err.Error())
	}
}

// handleListDatabaseVersions handles GET /api/v1/database-versions?engine=.
// Without engine it returns every engine's list.
func (rt *Router) handleListDatabaseVersions(w http.ResponseWriter, r *http.Request) {
	if !rt.versionsOr501(w) {
		return
	}
	if engine := strings.TrimSpace(r.URL.Query().Get("engine")); engine != "" {
		v, err := rt.dbVersions.Versions(engine)
		if err != nil {
			writeVersionsError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
		return
	}
	out := []dbupgrade.VersionsView{}
	for _, id := range rt.dbVersions.Engines() {
		if v, err := rt.dbVersions.Versions(id); err == nil {
			out = append(out, v)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"engines": out})
}

type liveVersionsResponse struct {
	dbupgrade.VersionsView
	Cooldown bool `json:"cooldown"`
}

// handleRefreshDatabaseVersions handles POST /api/v1/database-versions/refresh?engine=.
func (rt *Router) handleRefreshDatabaseVersions(w http.ResponseWriter, r *http.Request) {
	if !rt.versionsOr501(w) {
		return
	}
	engine := strings.TrimSpace(r.URL.Query().Get("engine"))
	if engine == "" {
		writeError(w, http.StatusBadRequest, "engine is required")
		return
	}
	v, cooled, err := rt.dbVersions.LiveRefresh(r.Context(), engine)
	if err != nil {
		writeVersionsError(w, err)
		return
	}
	if cooled {
		v.Message = "checked just now"
	}
	writeJSON(w, http.StatusOK, liveVersionsResponse{VersionsView: v, Cooldown: cooled})
}

// handleGetDatabaseCatalogStatus handles GET /api/v1/database-catalog.
func (rt *Router) handleGetDatabaseCatalogStatus(w http.ResponseWriter, _ *http.Request) {
	if !rt.versionsOr501(w) {
		return
	}
	writeJSON(w, http.StatusOK, rt.dbVersions.Status())
}

// handleRefreshDatabaseCatalog handles POST /api/v1/database-catalog/refresh.
func (rt *Router) handleRefreshDatabaseCatalog(w http.ResponseWriter, r *http.Request) {
	if !rt.versionsOr501(w) {
		return
	}
	if err := rt.dbVersions.Refresh(r.Context()); err != nil {
		writeVersionsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rt.dbVersions.Status())
}

type databaseCatalogSettings struct {
	RemoteEnabled *bool `json:"remote_enabled"`
}

// handlePutDatabaseCatalogSettings handles PUT /api/v1/settings/database-catalog.
func (rt *Router) handlePutDatabaseCatalogSettings(w http.ResponseWriter, r *http.Request) {
	if !rt.versionsOr501(w) {
		return
	}
	var req databaseCatalogSettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.RemoteEnabled == nil {
		writeError(w, http.StatusBadRequest, "remote_enabled is required")
		return
	}
	if err := rt.dbVersions.SetRemoteEnabled(*req.RemoteEnabled); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save the setting")
		return
	}
	writeJSON(w, http.StatusOK, rt.dbVersions.Status())
}
