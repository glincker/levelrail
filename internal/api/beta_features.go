package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

// Where a feature's on state comes from.
const (
	betaSourceEnv      = "env"
	betaSourceSettings = "setting"
	betaSourceOff      = "none"
)

type betaFeatureResource struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Summary         string `json:"summary"`
	Enabled         bool   `json:"enabled"`
	EnabledBy       string `json:"enabled_by"`
	Toggleable      bool   `json:"toggleable"`
	RequiresRestart bool   `json:"requires_restart"`
	EnvHint         string `json:"env_hint"`
	DocsPath        string `json:"docs_path"`
	Maturity        string `json:"maturity"`
	// Locked is true when APP_EXPERIMENTAL turns it on, so Settings cannot turn it off.
	Locked bool `json:"locked"`
}

type betaFeatureRequest struct {
	Enabled *bool `json:"enabled"`
}

// BetaFeatureStore persists the Settings switches for beta features.
type BetaFeatureStore interface {
	ListBetaFeatures(ctx context.Context) (map[string]bool, error)
}

// LoadBetaFeatures applies stored Settings switches at startup.
func LoadBetaFeatures(ctx context.Context, s BetaFeatureStore) error {
	stored, err := s.ListBetaFeatures(ctx)
	if err != nil {
		return fmt.Errorf("load beta features: %w", err)
	}
	for _, f := range experimental.ToggleableList() {
		experimental.SetRuntime(f, stored[string(f)])
	}
	return nil
}

func betaFeatureState(f experimental.Feature) betaFeatureResource {
	info := experimental.Describe(f)
	out := betaFeatureResource{
		ID: string(f), Name: info.Name, Summary: info.Summary, Enabled: experimental.Enabled(f),
		EnabledBy: betaSourceOff, Toggleable: experimental.Toggleable(f), RequiresRestart: info.RestartRequired,
		EnvHint: experimental.EnvHint(f), DocsPath: info.DocsPath, Maturity: info.Maturity,
	}
	switch {
	case experimental.EnabledByEnv(f):
		out.EnabledBy, out.Locked = betaSourceEnv, true
	case out.Enabled:
		out.EnabledBy = betaSourceSettings
	}
	return out
}

// handleListBetaFeatures serves GET /api/v1/settings/beta-features, every gated feature.
func (rt *Router) handleListBetaFeatures(w http.ResponseWriter, _ *http.Request) {
	out := []betaFeatureResource{}
	for _, f := range experimental.All() {
		out = append(out, betaFeatureState(f))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetBetaFeature serves PUT /api/v1/settings/beta-features/{feature}.
func (rt *Router) handleSetBetaFeature(w http.ResponseWriter, r *http.Request) {
	f := experimental.Feature(r.PathValue("feature"))
	if !slices.Contains(experimental.All(), f) {
		writeError(w, http.StatusNotFound, "no gated feature named "+string(f))
		return
	}
	if !experimental.Toggleable(f) {
		writeError(w, http.StatusConflict, string(f)+" is wired at startup and cannot be switched at runtime. "+experimental.EnvHint(f))
		return
	}
	var req betaFeatureRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
		writeError(w, http.StatusBadRequest, `body must be {"enabled": true} or {"enabled": false}`)
		return
	}
	s := rt.advisorStore()
	if s == nil {
		writeError(w, http.StatusNotImplemented, "beta feature storage is not configured")
		return
	}
	actor := rt.eventActor(r)
	if err := s.SetBetaFeature(r.Context(), string(f), *req.Enabled, actor, time.Now()); err != nil {
		rt.internalError(w, "api: set beta feature", err, slog.String("feature", string(f)))
		return
	}
	experimental.SetRuntime(f, *req.Enabled)
	rt.advisor.invalidate()
	rt.logger.Info("beta feature switched", slog.String("feature", string(f)), slog.Bool("enabled", *req.Enabled), slog.String("actor", actor))
	writeJSON(w, http.StatusOK, betaFeatureState(f))
}
