package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

// Where a beta feature's state comes from.
const (
	betaSourceEnv      = "env"
	betaSourceSettings = "settings"
	betaSourceOff      = "off"
)

type betaFeatureResource struct {
	Feature string `json:"feature"`
	Enabled bool   `json:"enabled"`
	Source  string `json:"source"`
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
	out := betaFeatureResource{Feature: string(f), Enabled: experimental.Enabled(f), Source: betaSourceOff}
	switch {
	case experimental.EnabledByEnv(f):
		out.Source, out.Locked = betaSourceEnv, true
	case out.Enabled:
		out.Source = betaSourceSettings
	}
	return out
}

// handleListBetaFeatures serves GET /api/v1/settings/beta-features.
func (rt *Router) handleListBetaFeatures(w http.ResponseWriter, _ *http.Request) {
	out := []betaFeatureResource{}
	for _, f := range experimental.ToggleableList() {
		out = append(out, betaFeatureState(f))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetBetaFeature serves PUT /api/v1/settings/beta-features/{feature}.
func (rt *Router) handleSetBetaFeature(w http.ResponseWriter, r *http.Request) {
	f := experimental.Feature(r.PathValue("feature"))
	if !experimental.Toggleable(f) {
		writeError(w, http.StatusNotFound, "no beta feature named "+string(f))
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
