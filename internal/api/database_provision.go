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
	if _, ok := rt.pendingDatabaseOrWrite(w, r, name); !ok {
		return
	}
	choice := ""
	switch req.Fix {
	case dbprovision.FixRemoveVolume:
		if !req.Confirm {
			writeError(w, http.StatusBadRequest, "removing the leftover volume deletes its data: send confirm true to proceed")
			return
		}
		choice = existingVolumeDiscard
	case dbprovision.FixReuseVolume:
		choice = existingVolumeReuse
	}
	if choice != "" {
		s, ok := rt.databases.(existingVolumeSetter)
		if !ok {
			writeError(w, http.StatusNotImplemented, "storing the volume choice is not available")
			return
		}
		if err := s.SetDatabaseExistingVolume(r.Context(), name, choice); err != nil {
			rt.internalError(w, "api: retry provisioning: store volume choice failed", err, slog.String("name", name))
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
