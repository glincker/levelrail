package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
	"github.com/GLINCKER/levelrail/internal/profile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// MeshSettingsStore persists the operator's mesh on/off choice; *store.DB satisfies it.
type MeshSettingsStore interface {
	GetMeshSetting(ctx context.Context) (store.MeshSetting, error)
	SetMeshSetting(ctx context.Context, enabled bool, actor string, now time.Time) error
}

// meshSettingsState is what the mesh settings routes need beyond the store.
type meshSettingsState struct {
	store     MeshSettingsStore
	lookupEnv func(string) (string, bool)
	readiness func(meshRunning bool) network.ReadinessEnv
	startErr  string
}

// WithMeshSettings enables GET/PUT /api/v1/mesh/settings.
func WithMeshSettings(s MeshSettingsStore) Option {
	return func(rt *Router) { rt.meshSettings.store = s }
}

// SetMeshStartError records why the mesh failed to start, so the settings
// card can show it instead of a bare "restart required".
func (rt *Router) SetMeshStartError(err error) {
	if err != nil {
		rt.meshSettings.startErr = err.Error()
	}
}

type meshSettingsResource struct {
	// Configured is the choice that applies on next start; Effective is
	// whether the mesh is running in this process now.
	Configured      bool   `json:"configured"`
	Effective       bool   `json:"effective"`
	Source          string `json:"source"`
	EnvVar          string `json:"env_var"`
	RestartRequired bool   `json:"restart_required"`
	RestartHint     string `json:"restart_hint,omitempty"`
	RemoteNodes     int    `json:"remote_nodes"`
	// AgentAddr is the host:port new agents dial (the advertise host plus the
	// agent port); it must match the agent certificate SAN. Empty when unset.
	AgentAddr  string          `json:"agent_addr,omitempty"`
	Checks     []network.Check `json:"checks"`
	CanEnable  bool            `json:"can_enable"`
	StartError string          `json:"start_error,omitempty"`
	UpdatedBy  string          `json:"updated_by,omitempty"`
	UpdatedAt  *time.Time      `json:"updated_at,omitempty"`
}

type updateMeshSettingsRequest struct {
	Enabled bool `json:"enabled"`
	// Force allows disabling while other nodes are enrolled.
	Force bool `json:"force"`
}

func (rt *Router) meshSettingsLookup() func(string) (string, bool) {
	if rt.meshSettings.lookupEnv != nil {
		return rt.meshSettings.lookupEnv
	}
	return os.LookupEnv
}

func (rt *Router) meshReadinessEnv(running bool) network.ReadinessEnv {
	if rt.meshSettings.readiness != nil {
		return rt.meshSettings.readiness(running)
	}
	return network.SystemReadinessEnv(rt.advertiseHost(), running)
}

func (rt *Router) buildMeshSettings(ctx context.Context) (meshSettingsResource, error) {
	var saved store.MeshSetting
	if rt.meshSettings.store != nil {
		s, err := rt.meshSettings.store.GetMeshSetting(ctx)
		if err != nil {
			return meshSettingsResource{}, err
		}
		saved = s
	}
	configured, source := network.ResolveEnabledWithProfile(rt.meshSettingsLookup(), saved.Enabled, saved.Set, profile.For(profile.Active()).Mesh)
	running := rt.mesh != nil

	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		return meshSettingsResource{}, err
	}
	remote := 0
	for _, n := range nodes {
		if rt.localNodeID == "" || n.ID != rt.localNodeID {
			remote++
		}
	}

	checks := network.ReadinessChecks(rt.meshReadinessEnv(running))
	res := meshSettingsResource{
		Configured: configured, Effective: running, Source: source, EnvVar: network.EnvMeshEnabled,
		RestartRequired: configured != running,
		RemoteNodes:     remote,
		AgentAddr:       rt.defaultControlPlaneAddr(),
		Checks:          checks,
		CanEnable:       len(network.Blocking(checks)) == 0,
		UpdatedBy:       saved.UpdatedBy,
		StartError:      rt.meshSettings.startErr,
	}
	if !saved.UpdatedAt.IsZero() {
		t := saved.UpdatedAt
		res.UpdatedAt = &t
	}
	if res.RestartRequired {
		res.RestartHint = "sudo systemctl restart " + strings.ToLower(rt.brand.ShortName)
	}
	return res, nil
}

func (rt *Router) handleGetMeshSettings(w http.ResponseWriter, r *http.Request) {
	res, err := rt.buildMeshSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get mesh settings failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (rt *Router) handleUpdateMeshSettings(w http.ResponseWriter, r *http.Request) {
	if rt.meshSettings.store == nil {
		writeError(w, http.StatusNotImplemented, "mesh settings are not configured on this control plane")
		return
	}
	var req updateMeshSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	cur, err := rt.buildMeshSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: update mesh settings failed", err)
		return
	}
	if cur.Source == network.SettingSourceEnv {
		writeError(w, http.StatusConflict, "mesh is pinned by "+network.EnvMeshEnabled+"; unset it to manage the mesh here")
		return
	}
	if req.Enabled && !cur.CanEnable {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"error":  "this server cannot run the mesh yet: fix the failing checks and try again",
			"checks": cur.Checks,
		})
		return
	}
	if !req.Enabled && cur.RemoteNodes > 0 && !req.Force {
		writeError(w, http.StatusConflict, "other nodes are enrolled and reach this control plane over the mesh; disabling it disconnects them. Retry with force to confirm")
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	if err := rt.meshSettings.store.SetMeshSetting(r.Context(), req.Enabled, actor, time.Now()); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		rt.internalError(w, "api: save mesh setting failed", err)
		return
	}
	rt.logger.Info("api: mesh setting changed", slog.Bool("enabled", req.Enabled), slog.String("actor", actor))
	rt.handleGetMeshSettings(w, r)
}
