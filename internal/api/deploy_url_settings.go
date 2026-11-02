package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"

	"github.com/GLINCKER/levelrail/internal/deployurls"
)

// DeployURLSettingsStore is the store surface for the deploy URL copies mode.
// *store.DB satisfies it; a store without it leaves the routes at 501.
type DeployURLSettingsStore interface {
	GetDeployURLCopiesMode(ctx context.Context) (string, error)
	SetDeployURLCopiesMode(ctx context.Context, mode string) error
}

// deployURLCopiesResource is the wire shape of GET and PUT /api/v1/settings/deploy-url-copies.
type deployURLCopiesResource struct {
	// Mode is the effective mode: the stored value, else the env seed, else the default.
	Mode string `json:"mode"`
	// Source says where Mode came from: stored, env or default.
	Source  string   `json:"source"`
	Default string   `json:"default"`
	Modes   []string `json:"modes"`
}

// deployURLCopiesUpdate is the PUT body; an empty mode clears the stored value.
type deployURLCopiesUpdate struct {
	Mode string `json:"mode"`
}

func deployURLCopiesView(stored string) deployURLCopiesResource {
	res := deployURLCopiesResource{
		Mode: deployurls.DefaultMode, Source: "default", Default: deployurls.DefaultMode,
		Modes: []string{deployurls.ModeOff, deployurls.ModePrivate, deployurls.ModePublic},
	}
	if m, ok := deployurls.ParseMode(os.Getenv(deployurls.EnvCopies)); ok {
		res.Mode, res.Source = m, "env"
	}
	if m, ok := deployurls.ParseMode(stored); ok {
		res.Mode, res.Source = m, "stored"
	}
	return res
}

// handleGetDeployURLCopies handles GET /api/v1/settings/deploy-url-copies.
func (rt *Router) handleGetDeployURLCopies(w http.ResponseWriter, r *http.Request) {
	if rt.deployURLSettings == nil {
		writeError(w, http.StatusNotImplemented, "deploy url settings are not configured on this control plane")
		return
	}
	stored, err := rt.deployURLSettings.GetDeployURLCopiesMode(r.Context())
	if err != nil {
		rt.internalError(w, "api: get deploy url copies mode failed", err)
		return
	}
	writeJSON(w, http.StatusOK, deployURLCopiesView(stored))
}

// handlePutDeployURLCopies handles PUT /api/v1/settings/deploy-url-copies.
// Root only: private keeps old deploys off the public internet, public opens them.
func (rt *Router) handlePutDeployURLCopies(w http.ResponseWriter, r *http.Request) {
	if rt.deployURLSettings == nil {
		writeError(w, http.StatusNotImplemented, "deploy url settings are not configured on this control plane")
		return
	}
	var req deployURLCopiesUpdate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mode := ""
	if req.Mode != "" {
		m, ok := deployurls.ParseMode(req.Mode)
		if !ok {
			writeError(w, http.StatusBadRequest, "mode must be off, private or public, or empty to inherit the default")
			return
		}
		mode = m
	}
	if err := rt.deployURLSettings.SetDeployURLCopiesMode(r.Context(), mode); err != nil {
		rt.internalError(w, "api: set deploy url copies mode failed", err)
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, deployURLCopiesView(mode))
}

func deployURLSettingsOf(s any) DeployURLSettingsStore {
	d, _ := s.(DeployURLSettingsStore)
	return d
}
