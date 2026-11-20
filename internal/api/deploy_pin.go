package api

import (
	"context"
	"log/slog"
	"net/http"
)

// deployPinStore is the optional deployAttempts capability behind deploy pins.
type deployPinStore interface {
	PinDeploy(ctx context.Context, serviceName, attemptID, by string) error
	UnpinDeploy(ctx context.Context, attemptID string) error
	ListPinnedDeploys(ctx context.Context, serviceName string) (map[string]bool, error)
}

// pinnedDeploys returns the pinned attempt ids of an app; a failure reads as none pinned for display.
func (rt *Router) pinnedDeploys(ctx context.Context, app string) map[string]bool {
	s, ok := rt.deployAttempts.(deployPinStore)
	if !ok {
		return nil
	}
	pins, err := s.ListPinnedDeploys(ctx, app)
	if err != nil {
		rt.logger.Warn("api: list pinned deploys failed", slog.String("error", err.Error()), slog.String("name", app))
		return nil
	}
	return pins
}

// handlePinDeploy handles PUT /api/v1/apps/{name}/deploys/{deployId}/pin:
// image retention and the janitor keep this deploy and its image.
func (rt *Router) handlePinDeploy(w http.ResponseWriter, r *http.Request) {
	rt.setDeployPin(w, r, true)
}

// handleUnpinDeploy handles DELETE /api/v1/apps/{name}/deploys/{deployId}/pin.
func (rt *Router) handleUnpinDeploy(w http.ResponseWriter, r *http.Request) {
	rt.setDeployPin(w, r, false)
}

func (rt *Router) setDeployPin(w http.ResponseWriter, r *http.Request, pin bool) {
	s, ok := rt.deployAttempts.(deployPinStore)
	if !ok {
		writeError(w, http.StatusNotImplemented, "deploy pins are not configured on this control plane")
		return
	}
	a, ok := rt.loadAppAttempt(w, r)
	if !ok {
		return
	}
	var err error
	if pin {
		err = s.PinDeploy(r.Context(), a.ServiceName, a.ID, rt.eventActor(r))
	} else {
		err = s.UnpinDeploy(r.Context(), a.ID)
	}
	if err != nil {
		rt.internalError(w, "api: set deploy pin failed", err, slog.String("attempt_id", a.ID))
		return
	}
	rt.writeAttempt(w, r, a.ID)
}
