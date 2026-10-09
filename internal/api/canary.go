package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	canaryDefaultWeight = 10
	canaryMaxWeight     = 99
)

// CanaryStore is the store surface the canary routes need.
type CanaryStore interface {
	SetCanaryRelease(ctx context.Context, c store.CanaryRelease) error
	GetCanaryRelease(ctx context.Context, serviceName string) (store.CanaryRelease, bool, error)
	DeleteCanaryRelease(ctx context.Context, serviceName string) error
	ListCanaryReleases(ctx context.Context) ([]store.CanaryRelease, error)
}

type canaryResource struct {
	Active    bool       `json:"active"`
	Image     string     `json:"image,omitempty"`
	Weight    int        `json:"weight"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

type canaryRequest struct {
	Image  string `json:"image"`
	Weight *int   `json:"weight,omitempty"`
}

func toCanaryResource(c store.CanaryRelease, ok bool) canaryResource {
	if !ok {
		return canaryResource{}
	}
	return canaryResource{Active: true, Image: c.Image, Weight: c.Weight, CreatedAt: &c.CreatedAt}
}

func validCanaryWeight(w int) bool { return w >= 0 && w <= canaryMaxWeight }

func (rt *Router) handleGetCanary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, ok, err := rt.canaries.GetCanaryRelease(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: get canary", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, toCanaryResource(c, ok))
}

// handleStartCanary clones the app into a hidden "<app>--canary" service
// running the new image. Ingress then sends weight percent of requests to it.
func (rt *Router) handleStartCanary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req canaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Image = strings.TrimSpace(req.Image)
	weight := canaryDefaultWeight
	if req.Weight != nil {
		weight = *req.Weight
	}
	if req.Image == "" {
		writeError(w, http.StatusBadRequest, "image is required")
		return
	}
	if weight < 1 || weight > canaryMaxWeight {
		writeError(w, http.StatusBadRequest, "weight must be between 1 and 99")
		return
	}
	if store.IsCanaryService(name) {
		writeError(w, http.StatusBadRequest, "a canary cannot have its own canary")
		return
	}
	source, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: start canary: load app", err, slog.String("name", name))
		return
	}
	if reason := rt.canaryBlockReason(r.Context(), *source); reason != "" {
		writeError(w, http.StatusConflict, reason)
		return
	}
	if _, exists, err := rt.canaries.GetCanaryRelease(r.Context(), name); err != nil {
		rt.internalError(w, "api: start canary: check existing", err, slog.String("name", name))
		return
	} else if exists {
		writeError(w, http.StatusConflict, "a canary is already running for this app, promote or abort it first")
		return
	}

	if rt.imageResolver != nil {
		auth := rt.imageTrigger(r.Context(), *source, true, "canary start").Auth
		if _, err := rt.imageResolver.ResolveImage(r.Context(), req.Image, auth, true); err != nil {
			writeError(w, http.StatusBadRequest, "cannot resolve image "+req.Image+": "+err.Error())
			return
		}
	}

	canaryName := store.CanaryServiceName(name)
	clone := cloneDesiredService(*source, canaryName, nil)
	clone.Image = req.Image
	if err := rt.apps.SaveDesiredService(r.Context(), clone); err != nil {
		rt.internalError(w, "api: start canary: save clone", err, slog.String("name", name))
		return
	}
	if source.ProjectID != "" {
		if err := rt.apps.UpdateServiceProject(r.Context(), canaryName, source.ProjectID); err != nil {
			rt.rollbackClone(r.Context(), canaryName)
			rt.internalError(w, "api: start canary: assign project", err, slog.String("name", name))
			return
		}
	}
	if rt.secrets != nil {
		if err := rt.applyCloneExtras(r.Context(), *source, cloneAppRequest{NewName: canaryName, CopySecrets: true}); err != nil {
			rt.rollbackClone(r.Context(), canaryName)
			rt.internalError(w, "api: start canary: copy secrets", err, slog.String("name", name))
			return
		}
	}
	rel := store.CanaryRelease{ServiceName: name, CanaryService: canaryName, Image: req.Image, Weight: weight}
	if err := rt.canaries.SetCanaryRelease(r.Context(), rel); err != nil {
		rt.rollbackClone(r.Context(), canaryName)
		rt.internalError(w, "api: start canary: record", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	rel, _, _ = rt.canaries.GetCanaryRelease(r.Context(), name)
	writeJSON(w, http.StatusCreated, toCanaryResource(rel, true))
}

func (rt *Router) canaryBlockReason(ctx context.Context, svc store.DesiredService) string {
	switch {
	case svc.HostPort != nil:
		return "an app with a pinned host port cannot run two releases at once"
	case svc.Replicas > 1:
		return "canary needs a single-replica app"
	case svc.ImageIDRef != "":
		return "built from source, canary supports image apps"
	case svc.Suspended:
		return "app is stopped"
	case rt.hasRunningDeployAttempt(ctx, svc.Name):
		return "a deploy is already running"
	}
	if frozen, reason := rt.freezeStatus(ctx, svc.Name); frozen {
		return "deploy freeze active: " + reason
	}
	return ""
}

func (rt *Router) handleSetCanaryWeight(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var req canaryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Weight == nil || !validCanaryWeight(*req.Weight) {
		writeError(w, http.StatusBadRequest, "weight must be between 0 and 99 (0 pauses the canary)")
		return
	}
	c, ok, err := rt.canaries.GetCanaryRelease(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: set canary weight: load", err, slog.String("name", name))
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no canary is running for this app")
		return
	}
	c.Weight = *req.Weight
	if err := rt.canaries.SetCanaryRelease(r.Context(), c); err != nil {
		rt.internalError(w, "api: set canary weight: save", err, slog.String("name", name))
		return
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, toCanaryResource(c, true))
}

// handleAbortCanary removes the canary clone; all traffic returns to the stable release.
func (rt *Router) handleAbortCanary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if _, ok, err := rt.canaries.GetCanaryRelease(r.Context(), name); err != nil {
		rt.internalError(w, "api: abort canary: load", err, slog.String("name", name))
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "no canary is running for this app")
		return
	}
	if err := rt.removeCanary(r.Context(), name); err != nil {
		rt.internalError(w, "api: abort canary", err, slog.String("name", name))
		return
	}
	writeJSON(w, http.StatusOK, canaryResource{})
}

// handlePromoteCanary deploys the canary image to the stable app through the
// normal deploy path (readiness gate, rollback target), then removes the clone.
func (rt *Router) handlePromoteCanary(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	c, ok, err := rt.canaries.GetCanaryRelease(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: promote canary: load", err, slog.String("name", name))
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no canary is running for this app")
		return
	}
	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if err != nil {
		rt.internalError(w, "api: promote canary: load app", err, slog.String("name", name))
		return
	}
	if reason := rt.imageUpdateSkipReason(r.Context(), *svc); reason != "" {
		writeError(w, http.StatusConflict, reason)
		return
	}
	trigger := rt.imageTrigger(r.Context(), *svc, true, "canary promote")
	trigger.Source = store.DeployAttemptSourceImage
	trigger.Ordered, trigger.Sequencer = nil, nil
	updated, err := trigger.Deploy(r.Context(), *svc, c.Image)
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	if rt.deployNotifier != nil {
		rt.deployNotifier.Dispatch(r.Context(), resourceIDForApp(name), alerting.DeployOutcome{AppName: name, Image: updated.Image, Succeeded: true})
	}
	if err := rt.removeCanary(r.Context(), name); err != nil {
		rt.logger.Error("api: promote canary: cleanup failed", slog.String("name", name), slog.String("error", err.Error()))
	}
	writeJSON(w, http.StatusOK, canaryResource{})
}

func (rt *Router) removeCanary(ctx context.Context, name string) error {
	if err := rt.canaries.DeleteCanaryRelease(ctx, name); err != nil {
		return err
	}
	rt.rollbackClone(ctx, store.CanaryServiceName(name))
	rt.nudgeReconciler()
	return nil
}
