package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/pages"
)

// pagesNotifyChannel is an attached channel; the destination URL and secret are never returned.
type pagesNotifyChannel struct {
	TargetID  string `json:"target_id"`
	ChannelID string `json:"channel_id"`
	Enabled   bool   `json:"enabled"`
}

type pagesNotifyResource struct {
	Site     string               `json:"site"`
	Enabled  bool                 `json:"enabled"`
	Events   []string             `json:"events"`
	Explicit bool                 `json:"explicit"`
	Channels []pagesNotifyChannel `json:"channels"`
}

type pagesNotifyRequest struct {
	Enabled   *bool    `json:"enabled"`
	Events    []string `json:"events"`
	ChannelID string   `json:"channel_id"`
}

// pagesNotifyDeps resolves the site and the stores; the filter store rides on the alerting DB.
func (rt *Router) pagesNotifyDeps(w http.ResponseWriter, r *http.Request) (string, *pages.NotifyStore, bool) {
	exec, ok := rt.deployNotifyTargets.(pages.SQLExecer)
	if !ok || rt.deployNotifyTargets == nil {
		writeError(w, http.StatusNotImplemented, "deploy notifications are not configured on this control plane")
		return "", nil, false
	}
	site, _, ok := rt.staticSettingsDeps(w, r)
	if !ok {
		return "", nil, false
	}
	return site.Name, pages.NewNotifyStore(exec), true
}

func (rt *Router) pagesNotifyView(r *http.Request, name string, store *pages.NotifyStore) (pagesNotifyResource, error) {
	cfg, explicit, err := store.Get(r.Context(), resourceIDForApp(name))
	if err != nil {
		return pagesNotifyResource{}, err
	}
	targets, err := rt.deployNotifyTargets.ListDeployTargetsForResource(r.Context(), resourceIDForApp(name))
	if err != nil {
		return pagesNotifyResource{}, err
	}
	out := pagesNotifyResource{Site: name, Enabled: cfg.Enabled, Events: cfg.Events(), Explicit: explicit, Channels: []pagesNotifyChannel{}}
	for _, t := range targets {
		out.Channels = append(out.Channels, pagesNotifyChannel{TargetID: t.ID, ChannelID: t.ChannelID, Enabled: t.Enabled})
	}
	return out, nil
}

// handleGetPagesNotify handles GET /api/v1/static-sites/{name}/notify.
func (rt *Router) handleGetPagesNotify(w http.ResponseWriter, r *http.Request) {
	name, store, ok := rt.pagesNotifyDeps(w, r)
	if !ok {
		return
	}
	view, err := rt.pagesNotifyView(r, name, store)
	if err != nil {
		rt.internalError(w, "api: pages notify: read", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// handlePutPagesNotify handles PUT /api/v1/static-sites/{name}/notify: switch, event filter, and an optional channel to attach.
func (rt *Router) handlePutPagesNotify(w http.ResponseWriter, r *http.Request) {
	name, store, ok := rt.pagesNotifyDeps(w, r)
	if !ok {
		return
	}
	var req pagesNotifyRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cfg, _, err := store.Get(r.Context(), resourceIDForApp(name))
	if err != nil {
		rt.internalError(w, "api: pages notify: read", err, slog.String("name", name))
		return
	}
	cfg.Enabled = true
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.Events != nil {
		if cfg.OnSuccess, cfg.OnFailure, err = pages.ParseEvents(req.Events); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if req.ChannelID != "" && !rt.attachPagesNotifyChannel(w, r, name, req.ChannelID) {
		return
	}
	if err := store.Put(r.Context(), resourceIDForApp(name), cfg); err != nil {
		rt.internalError(w, "api: pages notify: save", err, slog.String("name", name))
		return
	}
	view, err := rt.pagesNotifyView(r, name, store)
	if err != nil {
		rt.internalError(w, "api: pages notify: reload", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// attachPagesNotifyChannel reuses the app deploy-notify target table, so channel secrets and the destination guard stay in the channel service.
func (rt *Router) attachPagesNotifyChannel(w http.ResponseWriter, r *http.Request, name, channelID string) bool {
	if rt.notificationChannels == nil {
		writeError(w, http.StatusNotImplemented, "notification channels are not configured on this control plane")
		return false
	}
	if _, err := rt.notificationChannels.GetNotificationChannel(r.Context(), channelID); errors.Is(err, alerting.ErrNotificationChannelNotFound) {
		writeError(w, http.StatusBadRequest, "unknown channel_id")
		return false
	} else if err != nil {
		rt.internalError(w, "api: pages notify: look up channel", err, slog.String("channel_id", channelID))
		return false
	}
	existing, err := rt.deployNotifyTargets.ListDeployTargetsForResource(r.Context(), resourceIDForApp(name))
	if err != nil {
		rt.internalError(w, "api: pages notify: list targets", err, slog.String("name", name))
		return false
	}
	for _, t := range existing {
		if t.ChannelID == channelID {
			return true
		}
	}
	id, err := alerting.NewDeployTargetID()
	if err != nil {
		rt.internalError(w, "api: pages notify: target id", err, slog.String("name", name))
		return false
	}
	t := alerting.DeployTarget{ID: id, ResourceID: resourceIDForApp(name), ChannelID: channelID, Enabled: true}
	if err := rt.deployNotifyTargets.SaveDeployTarget(r.Context(), t); err != nil {
		rt.internalError(w, "api: pages notify: attach channel", err, slog.String("name", name))
		return false
	}
	return true
}

// handleDeletePagesNotify handles DELETE /api/v1/static-sites/{name}/notify: back to notifying on every outcome.
func (rt *Router) handleDeletePagesNotify(w http.ResponseWriter, r *http.Request) {
	name, store, ok := rt.pagesNotifyDeps(w, r)
	if !ok {
		return
	}
	if err := store.Delete(r.Context(), resourceIDForApp(name)); err != nil {
		rt.internalError(w, "api: pages notify: reset", err, slog.String("name", name))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
