package api

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/localmode"
)

type localModeResource struct {
	Enabled bool `json:"enabled"`
	// DataDir is where this local instance keeps everything a reset deletes.
	DataDir string `json:"data_dir,omitempty"`
	// ResetSupported is false when no supervisor would relaunch the process after a reset.
	ResetSupported bool   `json:"reset_supported"`
	ResetHint      string `json:"reset_hint,omitempty"`
}

func (rt *Router) localModeResource() localModeResource {
	if !localmode.Enabled(os.Getenv) {
		return localModeResource{}
	}
	res := localModeResource{Enabled: true, DataDir: rt.dataDir}
	rt.restart.mu.Lock()
	supported := rt.restart.r.Supported()
	rt.restart.mu.Unlock()
	res.ResetSupported = supported && rt.dataDir != ""
	if !res.ResetSupported {
		res.ResetHint = "start this instance with the dev command so it can relaunch itself after a reset"
	}
	return res
}

func (rt *Router) handleGetLocalMode(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, rt.localModeResource())
}

// handleLocalModeReset asks the supervising dev command to wipe the data dir and relaunch; it never deletes anything itself.
func (rt *Router) handleLocalModeReset(w http.ResponseWriter, r *http.Request) {
	res := rt.localModeResource()
	if !res.Enabled {
		writeError(w, http.StatusNotFound, "this is not a local instance")
		return
	}
	if !res.ResetSupported {
		writeJSON(w, http.StatusConflict, res)
		return
	}
	if err := localmode.RequestReset(rt.dataDir); err != nil {
		rt.logger.Error("api: local reset marker not written", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "could not record the reset request")
		return
	}
	rt.restart.mu.Lock()
	trigger := rt.restart.r.Trigger
	rt.restart.pending = true
	rt.restart.mu.Unlock()
	_, _, actor, _ := rt.currentActor(r)
	rt.logger.Warn("api: local instance reset requested", slog.String("actor", actor))
	writeJSON(w, http.StatusAccepted, res)
	time.AfterFunc(restartTriggerDelay, trigger)
}

// localAppURL is the plain-HTTP address of a .localhost app on a local instance.
func (rt *Router) localAppURL(host string) (string, bool) {
	if !localmode.Enabled(os.Getenv) || !strings.HasSuffix(strings.ToLower(host), "."+ingress.LocalHost) {
		return "", false
	}
	port := rt.doctorHTTPPort
	if port == 0 {
		port = 80
	}
	return localmode.AppURL(host, port), true
}
