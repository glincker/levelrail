package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// databasePoolerStore is asserted on the database store so test fakes of
// DatabaseStore keep compiling without a pooler method.
type databasePoolerStore interface {
	SetDatabasePooler(ctx context.Context, name string, enabled bool) error
	SetDatabasePoolerTuning(ctx context.Context, name string, poolSize, maxClientConn int) error
}

// databasePoolerRequest is the optional POST body; 0 or absent keeps the env default.
type databasePoolerRequest struct {
	PoolSize      int `json:"pool_size"`
	MaxClientConn int `json:"max_client_conn"`
}

// databasePoolerResource is GET .../pooler's body. URLs carry a password
// placeholder, never the real credential.
type databasePoolerResource struct {
	Enabled         bool   `json:"enabled"`
	PoolSize        int    `json:"pool_size,omitempty"`
	MaxClientConn   int    `json:"max_client_conn,omitempty"`
	Supported       bool   `json:"supported"`
	ImageConfigured bool   `json:"image_configured"`
	Container       string `json:"container,omitempty"`
	PooledURL       string `json:"pooled_url,omitempty"`
	DirectURL       string `json:"direct_url,omitempty"`
	Status          string `json:"status,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Message         string `json:"message,omitempty"`
}

const poolerPasswordPlaceholder = "<password>"

func poolerURL(name string, port int) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s", name, poolerPasswordPlaceholder, database.ContainerName(name), port, name)
}

func (rt *Router) poolerStatus(ctx context.Context, d *store.DesiredDatabase) databasePoolerResource {
	res := databasePoolerResource{
		Enabled:         d.PoolerEnabled,
		PoolSize:        d.PoolerPoolSize,
		MaxClientConn:   d.PoolerMaxClientConn,
		Supported:       d.Engine == store.EnginePostgres,
		ImageConfigured: database.PoolerImage() != "",
	}
	if !res.Supported {
		return res
	}
	port, _ := database.ContainerPort(d.Engine)
	res.DirectURL = poolerURL(d.Name, port)
	if !d.PoolerEnabled {
		return res
	}
	res.Container = database.PoolerContainerName(d.Name)
	res.PooledURL = poolerURL(d.Name, database.PoolerPort)
	if rt.deploys == nil {
		return res
	}
	ctrl := databaseControllerName(d.Name)
	conds, err := rt.deploys.GetConditionsForControllers(ctx, []string{ctrl})
	if err != nil {
		rt.logger.Warn("api: pooler status: load conditions failed", slog.String("name", d.Name), slog.String("error", err.Error()))
		return res
	}
	for _, c := range conds[ctrl] {
		if c.Type == database.PoolerConditionType {
			res.Status, res.Reason, res.Message = poolerStatusWord(c.Status), c.Reason, c.Message
		}
	}
	return res
}

func poolerStatusWord(s reconcile.ConditionStatus) string {
	switch s {
	case reconcile.ConditionTrue:
		return "running"
	case reconcile.ConditionFalse:
		return "failed"
	default:
		return "unknown"
	}
}

// handleGetDatabasePooler handles GET /api/v1/databases/{name}/pooler.
func (rt *Router) handleGetDatabasePooler(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.loadDatabaseForRunner(w, r, r.PathValue("name"), "api: get pooler: load database failed")
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.poolerStatus(r.Context(), d))
}

// handleEnableDatabasePooler handles POST /api/v1/databases/{name}/pooler.
func (rt *Router) handleEnableDatabasePooler(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	d, ok := rt.loadDatabaseForRunner(w, r, name, "api: enable pooler: load database failed")
	if !ok {
		return
	}
	if d.Engine != store.EnginePostgres {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("the connection pooler is only supported for postgres, not %q", d.Engine))
		return
	}
	if database.PoolerImage() == "" {
		writeError(w, http.StatusBadRequest, "no pooler image is configured: set "+database.EnvPoolerImage+" on the control plane to a pinned tag or digest")
		return
	}
	var req databasePoolerRequest
	if r.Body != nil {
		if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if req.PoolSize < 0 || req.MaxClientConn < 0 {
		writeError(w, http.StatusBadRequest, "pool_size and max_client_conn must be positive, or 0 for the default")
		return
	}
	if req.PoolSize > 0 && req.MaxClientConn > 0 && req.PoolSize > req.MaxClientConn {
		writeError(w, http.StatusBadRequest, "pool_size cannot exceed max_client_conn")
		return
	}
	rt.setDatabasePoolerTuned(w, r, name, true, &req)
}

// handleDisableDatabasePooler handles DELETE /api/v1/databases/{name}/pooler.
func (rt *Router) handleDisableDatabasePooler(w http.ResponseWriter, r *http.Request) {
	rt.setDatabasePooler(w, r, r.PathValue("name"), false)
}

func (rt *Router) setDatabasePooler(w http.ResponseWriter, r *http.Request, name string, enabled bool) {
	rt.setDatabasePoolerTuned(w, r, name, enabled, nil)
}

func (rt *Router) setDatabasePoolerTuned(w http.ResponseWriter, r *http.Request, name string, enabled bool, tune *databasePoolerRequest) {
	ps, ok := rt.databases.(databasePoolerStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "this server's store does not support the connection pooler")
		return
	}
	if err := ps.SetDatabasePooler(r.Context(), name, enabled); errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set pooler failed", err, slog.String("name", name))
		return
	}
	if tune != nil {
		if err := ps.SetDatabasePoolerTuning(r.Context(), name, tune.PoolSize, tune.MaxClientConn); err != nil {
			rt.internalError(w, "api: set pooler tuning failed", err, slog.String("name", name))
			return
		}
	}
	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set pooler: reload failed", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, rt.poolerStatus(r.Context(), d))
}
