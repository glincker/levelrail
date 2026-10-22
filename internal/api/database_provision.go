package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
)

const envProvisionStreamInterval = "APP_DB_PROVISION_STREAM_INTERVAL"

func (rt *Router) registerDatabaseProvisionRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/databases/{name}/provision", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetDatabaseProvision))
	mux.HandleFunc("GET /api/v1/databases/{name}/provision/stream", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.withStreamReauth(databaseResourceFromPath, rt.handleDatabaseProvisionStream)))
	mux.HandleFunc("POST /api/v1/databases/{name}/provision/retry", rt.requireAbilityForResource(AbilityWrite, databaseResourceFromPath, rt.handleRetryDatabaseProvision))
}

func (rt *Router) provisionStore() dbprovision.Store {
	s, _ := rt.databases.(dbprovision.Store)
	return s
}

// beginProvision opens a provisioning attempt; a failure to record it never blocks creation.
func (rt *Router) beginProvision(ctx context.Context, name string) {
	s := rt.provisionStore()
	if s == nil {
		return
	}
	if _, err := dbprovision.NewRecorder(s).Begin(ctx, name); err != nil {
		rt.logger.Warn("api: begin database provisioning record failed", slog.String("error", err.Error()), slog.String("name", name))
	}
}

func (rt *Router) latestProvision(ctx context.Context, name string) (dbprovision.Attempt, bool, error) {
	s := rt.provisionStore()
	if s == nil {
		return dbprovision.Attempt{}, false, nil
	}
	id, events, err := s.LatestDatabaseProvisionAttempt(ctx, name)
	if err != nil || id == "" {
		return dbprovision.Attempt{}, false, err
	}
	return dbprovision.View(id, events), true, nil
}

func (rt *Router) handleGetDatabaseProvision(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	a, found, err := rt.latestProvision(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get database provisioning failed", err, slog.String("name", name))
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, typedError{Error: "no provisioning record for this database", Code: "no_provision", Message: "no provisioning record for this database"})
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (rt *Router) handleDatabaseProvisionStream(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if rt.provisionStore() == nil {
		writeError(w, http.StatusNotImplemented, "provisioning progress is not available")
		return
	}
	flusher, ok := rt.startLiveSSE(w, r)
	if !ok {
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	tick := time.NewTicker(durationEnv(envProvisionStreamInterval, time.Second))
	defer tick.Stop()
	var lastSent int
	var lastID string
	for {
		a, found, err := rt.latestProvision(r.Context(), name)
		if err == nil && found && (a.ID != lastID || len(a.Events) != lastSent) {
			lastID, lastSent = a.ID, len(a.Events)
			if data, mErr := json.Marshal(a); mErr == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
			if a.Status == dbprovision.AttemptReady {
				return
			}
		}
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
		}
	}
}

type retryProvisionRequest struct {
	Fix     string `json:"fix"`
	Confirm bool   `json:"confirm"`
}

func (rt *Router) handleRetryDatabaseProvision(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req retryProvisionRequest
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req)
	if rt.provisionStore() == nil {
		writeError(w, http.StatusNotImplemented, "provisioning progress is not available")
		return
	}
	if req.Fix == dbprovision.FixRemoveVolume {
		if !req.Confirm {
			writeError(w, http.StatusBadRequest, "removing the leftover volume deletes its data: send confirm true to proceed")
			return
		}
		if !rt.removeLeftoverVolumes(w, r, name) {
			return
		}
	}
	rt.beginProvision(r.Context(), name)
	rt.nudgeReconciler()
	a, _, err := rt.latestProvision(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: retry database provisioning failed", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusAccepted, a)
}

func (rt *Router) removeLeftoverVolumes(w http.ResponseWriter, r *http.Request, name string) bool {
	d, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: retry provisioning: load database failed", err, slog.String("name", name))
		return false
	}
	found, err := rt.leftoverDatabaseVolumes(r.Context(), name, d.NodeID)
	if err != nil {
		rt.internalError(w, "api: retry provisioning: list volumes failed", err, slog.String("name", name))
		return false
	}
	for _, v := range found {
		if v.Mounted {
			writeError(w, http.StatusConflict, "volume "+v.Name+" is still attached to a container")
			return false
		}
	}
	for _, v := range found {
		if err := rt.orphanedVolumes.RemoveVolume(r.Context(), v.Name); err != nil {
			rt.internalError(w, "api: retry provisioning: remove volume failed", err, slog.String("name", name))
			return false
		}
	}
	return true
}
