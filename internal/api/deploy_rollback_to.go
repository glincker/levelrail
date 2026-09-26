package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// rollbackImageRef returns the image reference that redeploys exactly the
// content target recorded, or an HTTP status and message when it cannot.
func (rt *Router) rollbackImageRef(ctx context.Context, target store.DeployAttempt) (ref string, status int, msg string) {
	if target.Status != store.DeployAttemptStatusSucceeded || target.Image == "" {
		return "", http.StatusConflict, "only a succeeded deploy can be rolled back to"
	}
	digest := target.ImageDigest
	if digest == "" {
		return "", http.StatusConflict, "this deploy has no recorded image digest; roll back by image tag with POST /api/v1/apps/{name}/deploys instead"
	}
	switch target.DigestReason {
	case store.DigestReasonResolved, store.DigestReasonPinned:
		return docker.PinImageRef(docker.UnpinImageRef(target.Image), digest), 0, ""
	}
	inspector, ok := rt.imageResolver.(docker.ImageInspector)
	if !ok {
		return "", http.StatusNotImplemented, "cannot verify the local image on this control plane"
	}
	id, err := inspector.InspectImageID(ctx, target.Image)
	if err != nil {
		rt.logger.Error("api: rollback: inspect image failed", slog.String("error", err.Error()), slog.String("image", target.Image))
		return "", http.StatusBadGateway, "could not inspect the image on the node"
	}
	if id == "" {
		return "", http.StatusGone, fmt.Sprintf("image %s was garbage collected and can no longer be rolled back to; redeploy from source instead", target.Image)
	}
	if id != digest {
		return "", http.StatusConflict, fmt.Sprintf("tag %s now points at different content than this deploy recorded; redeploy from source instead", target.Image)
	}
	return target.Image, 0, ""
}

// handleRollbackToDeploy handles POST /api/v1/apps/{name}/deploys/{deployId}/rollback:
// redeploys the content a past succeeded deploy recorded, pinned by digest,
// through the same freeze and approval gates as a plain deploy.
func (rt *Router) handleRollbackToDeploy(w http.ResponseWriter, r *http.Request) {
	target, ok := rt.lookupAppAttempt(w, r)
	if !ok {
		return
	}
	var req deployTriggerRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	existing, err := rt.apps.GetDesiredService(r.Context(), target.ServiceName)
	if err != nil {
		rt.logger.Error("api: rollback: load app failed", slog.String("error", err.Error()), slog.String("name", target.ServiceName))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	ref, status, msg := rt.rollbackImageRef(r.Context(), *target)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	req.Image = ref
	rt.applyDeploy(w, r, *existing, req, target.ID)
}
