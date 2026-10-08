package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ImageAutoUpdateStore is the store surface for per-app image auto-update.
type ImageAutoUpdateStore interface {
	SetImageAutoUpdate(ctx context.Context, serviceName string, enabled bool) error
	GetImageAutoUpdate(ctx context.Context, serviceName string) (store.ImageAutoUpdate, error)
	RecordImageAutoUpdateCheck(ctx context.Context, serviceName, result string, at time.Time) error
}

// Results CheckImageUpdate records; the UI and CLI show them verbatim.
const (
	imageUpdateUpToDate   = "up to date"
	imageUpdateRedeployed = "redeployed with a newer image"
)

type imageAutoUpdateResource struct {
	Enabled       bool       `json:"enabled"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	LastResult    string     `json:"last_result,omitempty"`
}

type setImageAutoUpdateRequest struct {
	Enabled bool `json:"enabled"`
}

func toImageAutoUpdateResource(u store.ImageAutoUpdate) imageAutoUpdateResource {
	return imageAutoUpdateResource{Enabled: u.Enabled, LastCheckedAt: u.LastCheckedAt, LastResult: u.LastResult}
}

func (rt *Router) handleGetImageAutoUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: get image auto-update: load app", err, slog.String("name", name))
		return
	}
	u, err := rt.imageAutoUpdates.GetImageAutoUpdate(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get image auto-update", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, toImageAutoUpdateResource(u))
}

func (rt *Router) handleSetImageAutoUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req setImageAutoUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.internalError(w, "api: set image auto-update: load app", err, slog.String("name", name))
		return
	}
	if err := rt.imageAutoUpdates.SetImageAutoUpdate(r.Context(), name, req.Enabled); err != nil {
		rt.internalError(w, "api: set image auto-update", err, slog.String("name", name))
		return
	}
	u, err := rt.imageAutoUpdates.GetImageAutoUpdate(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set image auto-update: reload", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, toImageAutoUpdateResource(u))
}

// handleCheckImageAutoUpdate runs one update check now, regardless of the opt-in.
func (rt *Router) handleCheckImageAutoUpdate(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	result, err := rt.CheckImageUpdate(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Warn("api: image update check failed", slog.String("name", name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	u, err := rt.imageAutoUpdates.GetImageAutoUpdate(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: image update check: reload", err, slog.String("name", name))
		return
	}
	res := toImageAutoUpdateResource(u)
	res.LastResult = result
	writeJSON(w, http.StatusOK, res)
}

// CheckImageUpdate compares serviceName's tag against its registry and
// redeploys when the digest moved. Skips (with a reason, not an error) apps
// built from source, stopped, frozen, or in a protected environment: an
// unattended pull must never bypass the gates a manual deploy goes through.
func (rt *Router) CheckImageUpdate(ctx context.Context, serviceName string) (string, error) {
	result, err := rt.checkImageUpdate(ctx, serviceName)
	if err != nil {
		if errors.Is(err, store.ErrServiceNotFound) {
			return "", err
		}
		result = "check failed: " + err.Error()
	}
	if recErr := rt.imageAutoUpdates.RecordImageAutoUpdateCheck(ctx, serviceName, result, time.Now()); recErr != nil {
		rt.logger.Warn("api: record image update check failed", slog.String("name", serviceName), slog.String("error", recErr.Error()))
	}
	return result, err
}

func (rt *Router) checkImageUpdate(ctx context.Context, serviceName string) (string, error) {
	svc, err := rt.apps.GetDesiredService(ctx, serviceName)
	if err != nil {
		return "", err
	}
	if skip := rt.imageUpdateSkipReason(ctx, *svc); skip != "" {
		return "skipped: " + skip, nil
	}
	if rt.imageResolver == nil {
		return "", errors.New("no image resolver is configured on this control plane")
	}
	tagRef := docker.UnpinImageRef(svc.Image)
	trigger := rt.imageTrigger(ctx, *svc, true, "image auto-update")
	remote, err := rt.imageResolver.ResolveImage(ctx, tagRef, trigger.Auth, true)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", tagRef, err)
	}
	if remote.Digest == "" || remote.Digest == docker.ImageDigestOf(svc.Image) {
		return imageUpdateUpToDate, nil
	}
	trigger.Source = store.DeployAttemptSourceImage
	trigger.Ordered, trigger.Sequencer = nil, nil
	updated, err := trigger.Deploy(ctx, *svc, tagRef)
	if err != nil {
		return "", err
	}
	if rt.deployNotifier != nil {
		rt.deployNotifier.Dispatch(ctx, resourceIDForApp(svc.Name), alerting.DeployOutcome{AppName: svc.Name, Image: updated.Image, Succeeded: true})
	}
	return imageUpdateRedeployed, nil
}

func (rt *Router) imageUpdateSkipReason(ctx context.Context, svc store.DesiredService) string {
	switch {
	case svc.ImageIDRef != "":
		return "built from source, redeploy from git instead"
	case svc.Suspended:
		return "app is stopped"
	case rt.hasRunningDeployAttempt(ctx, svc.Name):
		return "a deploy is already running"
	}
	if frozen, reason := rt.freezeStatus(ctx, svc.Name); frozen {
		return "deploy freeze active: " + reason
	}
	if svc.EnvironmentID != "" {
		if env, err := rt.environments.GetEnvironment(ctx, svc.EnvironmentID); err == nil && env.Protected {
			return "protected environment, deploy it manually"
		}
	}
	return ""
}
