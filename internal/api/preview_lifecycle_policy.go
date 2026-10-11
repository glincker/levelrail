package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type forkSecretsApprovedKey struct{}

// withForkSecretsApproved marks one deploy as approved to give a fork the
// app's environment, the per-approval counterpart of allow_fork_secrets.
func withForkSecretsApproved(ctx context.Context) context.Context {
	return context.WithValue(ctx, forkSecretsApprovedKey{}, true)
}

func forkSecretsApproved(ctx context.Context) bool {
	v, _ := ctx.Value(forkSecretsApprovedKey{}).(bool)
	return v
}

// applyPreviewPolicyLifecycleFields validates and applies the lifecycle
// fields of a policy update onto s, returning a client-facing message for the
// first invalid value.
func (rt *Router) applyPreviewPolicyLifecycleFields(ctx context.Context, appName string, s *store.PreviewAppSettings, req setPreviewPolicyRequest) string {
	if req.MaxPreviews != nil {
		if *req.MaxPreviews < 0 || *req.MaxPreviews > 100 {
			return "max_previews must be between 0 and 100 (0 uses the platform cap)"
		}
		s.MaxPreviews = *req.MaxPreviews
	}
	if req.MemoryLimit != nil {
		m := strings.TrimSpace(*req.MemoryLimit)
		if m != "" && previewMemoryBytes(m) < 32<<20 {
			return "memory_limit must be a size of at least 32Mi such as 256Mi or 1Gi"
		}
		s.MemoryLimit = m
	}
	if req.CPULimit != nil {
		if *req.CPULimit < 0 || *req.CPULimit > 64 {
			return "cpu_limit must be between 0 and 64 (0 uses the platform default)"
		}
		s.CPULimit = *req.CPULimit
	}
	if req.IdleSleepMinutes != nil {
		if *req.IdleSleepMinutes != 0 && (*req.IdleSleepMinutes < minSleepIdleMinutes || *req.IdleSleepMinutes > maxSleepIdleMinutes) {
			return "idle_sleep_minutes must be 0 (platform default) or between 5 and 10080"
		}
		s.IdleSleepMinutes = *req.IdleSleepMinutes
	}
	if req.DatabaseStrategy != nil {
		if !store.ValidPreviewDatabaseStrategy(*req.DatabaseStrategy) {
			return "database_strategy must be none, shared, fresh or seed"
		}
		s.DatabaseStrategy = *req.DatabaseStrategy
	}
	if req.SeedDatabase != nil {
		s.SeedDatabase = strings.TrimSpace(*req.SeedDatabase)
	}
	if req.AllowForkSecrets != nil {
		s.AllowForkSecrets = *req.AllowForkSecrets
	}
	if req.AllowIndexing != nil {
		s.AllowIndexing = *req.AllowIndexing
	}
	return rt.applyPreviewGateFields(ctx, appName, s, req)
}

func (rt *Router) applyPreviewGateFields(ctx context.Context, appName string, s *store.PreviewAppSettings, req setPreviewPolicyRequest) string {
	if req.GateUsername != nil {
		s.GateUsername = strings.TrimSpace(*req.GateUsername)
	}
	if req.GatePassword != nil && *req.GatePassword != "" {
		if rt.secrets == nil {
			return "the basic auth gate needs a master key on this control plane"
		}
		if err := rt.secrets.SetValueGuarded(ctx, previewGateSecretPrefix+appName, previewGateSecretKey, *req.GatePassword, true); err != nil {
			rt.logger.Error("api: save preview gate password failed", slog.String("error", err.Error()), slog.String("name", appName))
			return "could not store the gate password"
		}
	}
	if req.GateBasicAuth == nil {
		return ""
	}
	if *req.GateBasicAuth {
		if s.GateUsername == "" {
			return "gate_username is required to turn the basic auth gate on"
		}
		exists := false
		if rt.secrets != nil {
			exists, _ = rt.secrets.Exists(ctx, previewGateSecretPrefix+appName, previewGateSecretKey)
		}
		if !exists {
			return "gate_password is required the first time the basic auth gate is turned on"
		}
	}
	s.GateBasicAuth = *req.GateBasicAuth
	return ""
}

type extendPreviewRequest struct {
	Hours int `json:"hours"`
}

// handleExtendPreviewEnvironment handles POST
// /api/v1/apps/{name}/previews/{number}/extend: pushes the preview's expiry
// out by hours from whichever is later, now or its current expiry.
func (rt *Router) handleExtendPreviewEnvironment(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ctx := r.Context()
	prNumber, err := strconv.Atoi(r.PathValue("number"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "pr number must be an integer")
		return
	}
	var req extendPreviewRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<10)).Decode(&req); err != nil || req.Hours < 1 || req.Hours > maxPreviewTTLHours {
		writeError(w, http.StatusBadRequest, "hours must be between 1 and "+strconv.Itoa(maxPreviewTTLHours))
		return
	}
	p, err := rt.previewEnvironments.GetPreviewEnvironmentByAppAndPR(ctx, name, prNumber)
	if errors.Is(err, store.ErrPreviewEnvironmentNotFound) {
		writeError(w, http.StatusNotFound, "no preview environment found for this pull request")
		return
	}
	if err != nil {
		rt.internalError(w, "api: extend preview: load", err, slog.String("name", name))
		return
	}
	ttl := rt.previewTTLFor(rt.previewSettings(ctx, name))
	now := time.Now().UTC()
	base := now
	if cur, ok := previewExpiry(*p, ttl); ok && cur.After(base) {
		base = cur
	}
	until := base.Add(time.Duration(req.Hours) * time.Hour)
	if limit := now.Add(maxPreviewTTLHours * time.Hour); until.After(limit) {
		until = limit
	}
	p.ExtendedUntil = until.Format(time.RFC3339Nano)
	if err := rt.previewEnvironments.SetPreviewEnvironmentExtendedUntil(ctx, p.ID, p.ExtendedUntil); err != nil {
		rt.internalError(w, "api: extend preview: save", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, rt.toPreviewEnvironmentResource(ctx, *p, ttl))
}
