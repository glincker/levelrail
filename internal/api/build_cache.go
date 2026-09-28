package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/build"
	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

const buildCacheOpTimeout = 2 * time.Minute

// BuildCacheSettingsStore persists build cache settings.
type BuildCacheSettingsStore interface {
	UpsertBuildCacheSetting(ctx context.Context, s store.BuildCacheSetting) error
	GetBuildCacheSetting(ctx context.Context, appName string) (store.BuildCacheSetting, error)
	ListBuildCacheSettings(ctx context.Context) ([]store.BuildCacheSetting, error)
	DeleteBuildCacheSetting(ctx context.Context, appName string) error
}

// BuildCacheService runs bucket-side cache hygiene.
type BuildCacheService interface {
	KeyPrefix(app string) (string, error)
	Stats(ctx context.Context, app string) (objectstore.CacheStats, error)
	Clear(ctx context.Context, app string) (objectstore.ClearResult, error)
}

type buildCacheResource struct {
	AppName       string `json:"app_name"`
	TargetID      string `json:"target_id"`
	Enabled       bool   `json:"enabled"`
	Mode          string `json:"mode"`
	KeyPrefix     string `json:"key_prefix,omitempty"`
	LastBuildAt   string `json:"last_build_at,omitempty"`
	LastResult    string `json:"last_result,omitempty"`
	LastWarning   string `json:"last_warning,omitempty"`
	LastClearedAt string `json:"last_cleared_at,omitempty"`
	UpdatedAt     string `json:"updated_at"`
}

func (rt *Router) toBuildCacheResource(d *StorageDeps, s store.BuildCacheSetting) buildCacheResource {
	res := buildCacheResource{
		AppName: s.AppName, TargetID: s.TargetID, Enabled: s.Enabled, Mode: s.Mode, LastBuildAt: s.LastBuildAt,
		LastResult: s.LastResult, LastWarning: s.LastWarning, LastClearedAt: s.LastClearedAt, UpdatedAt: s.UpdatedAt,
	}
	if s.AppName != "" {
		if prefix, err := d.BuildCache.KeyPrefix(s.AppName); err == nil {
			res.KeyPrefix = prefix
		}
	}
	return res
}

type buildCacheRequest struct {
	AppName  string `json:"app_name"`
	TargetID string `json:"target_id"`
	Mode     string `json:"mode,omitempty"`
	Enabled  *bool  `json:"enabled,omitempty"`
}

func (rt *Router) buildCacheOrNotImplemented(w http.ResponseWriter) (*StorageDeps, bool) {
	d, ok := rt.storageOrNotImplemented(w)
	if !ok {
		return nil, false
	}
	if d.BuildCache == nil || d.BuildCacheSettings == nil {
		writeError(w, http.StatusNotImplemented, "build cache is not configured on this control plane")
		return nil, false
	}
	return d, true
}

// handleListBuildCache handles GET /api/v1/build-cache: the global default
// (app_name empty) and every per-app setting. Credentials are never included.
func (rt *Router) handleListBuildCache(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.buildCacheOrNotImplemented(w)
	if !ok {
		return
	}
	settings, err := d.BuildCacheSettings.ListBuildCacheSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: list build cache settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]buildCacheResource, 0, len(settings))
	for _, s := range settings {
		out = append(out, rt.toBuildCacheResource(d, s))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleSetBuildCache handles PUT /api/v1/build-cache: create or replace the
// setting for an app, or the global default when app_name is empty.
func (rt *Router) handleSetBuildCache(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.buildCacheOrNotImplemented(w)
	if !ok {
		return
	}
	var req buildCacheRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mode := req.Mode
	if mode == "" {
		mode = build.CacheModeMax
	}
	if mode != build.CacheModeMin && mode != build.CacheModeMax {
		writeError(w, http.StatusBadRequest, "mode must be min or max")
		return
	}
	if req.TargetID == "" {
		writeError(w, http.StatusBadRequest, "target_id is required")
		return
	}
	if req.AppName != "" {
		if _, err := d.BuildCache.KeyPrefix(req.AppName); err != nil {
			writeError(w, http.StatusBadRequest, "app name cannot be used as a cache key prefix")
			return
		}
	}
	if _, ok := rt.loadStorageTarget(w, r, req.TargetID); !ok {
		return
	}
	if !rt.appExistsOrEmpty(w, r, req.AppName) {
		return
	}
	s := store.BuildCacheSetting{
		AppName: req.AppName, TargetID: req.TargetID, Enabled: req.Enabled == nil || *req.Enabled,
		Mode: mode, UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := d.BuildCacheSettings.UpsertBuildCacheSetting(r.Context(), s); err != nil {
		rt.logger.Error("api: set build cache failed", slog.String("error", err.Error()), slog.String("app", req.AppName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	saved, err := d.BuildCacheSettings.GetBuildCacheSetting(r.Context(), req.AppName)
	if err != nil {
		rt.logger.Error("api: reload build cache failed", slog.String("error", err.Error()), slog.String("app", req.AppName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, rt.toBuildCacheResource(d, saved))
}

// handleDeleteBuildCache handles DELETE /api/v1/build-cache?app=. It removes
// the setting only; bucket contents stay until cleared.
func (rt *Router) handleDeleteBuildCache(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.buildCacheOrNotImplemented(w)
	if !ok {
		return
	}
	err := d.BuildCacheSettings.DeleteBuildCacheSetting(r.Context(), r.URL.Query().Get("app"))
	if errors.Is(err, store.ErrBuildCacheSettingNotFound) {
		writeError(w, http.StatusNotFound, "build cache setting not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: delete build cache failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleBuildCacheStats handles GET /api/v1/build-cache/stats?app=: a bounded
// listing of the app's cache prefix in the bucket.
func (rt *Router) handleBuildCacheStats(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.buildCacheOrNotImplemented(w)
	if !ok {
		return
	}
	app := r.URL.Query().Get("app")
	if app == "" {
		writeError(w, http.StatusBadRequest, "app is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), buildCacheOpTimeout)
	defer cancel()
	stats, err := d.BuildCache.Stats(ctx, app)
	if rt.writeBuildCacheOpError(w, "stats", app, err) {
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

type buildCacheClearRequest struct {
	AppName string `json:"app_name"`
}

// handleClearBuildCache handles POST /api/v1/build-cache/clear: delete the
// app's cache objects, bounded per call. "more" true means run it again.
func (rt *Router) handleClearBuildCache(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.buildCacheOrNotImplemented(w)
	if !ok {
		return
	}
	var req buildCacheClearRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AppName == "" {
		writeError(w, http.StatusBadRequest, "app_name is required")
		return
	}
	if !rt.appExistsOrEmpty(w, r, req.AppName) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), buildCacheOpTimeout)
	defer cancel()
	res, err := d.BuildCache.Clear(ctx, req.AppName)
	if rt.writeBuildCacheOpError(w, "clear", req.AppName, err) {
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) writeBuildCacheOpError(w http.ResponseWriter, op, app string, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, objectstore.ErrBuildCacheNotConfigured):
		writeError(w, http.StatusConflict, "build cache is not configured for this app")
	case errors.Is(err, objectstore.ErrInvalidBuildCacheApp):
		writeError(w, http.StatusBadRequest, "invalid app name")
	default:
		rt.logger.Error("api: build cache "+op+" failed", slog.String("error", err.Error()), slog.String("app", app))
		writeError(w, http.StatusBadGateway, "storage destination request failed")
	}
	return true
}
