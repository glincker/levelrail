// Package rollbackref picks the image reference a rollback redeploys and
// verifies it still matches the content the target deploy recorded.
package rollbackref

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Resolve returns the reference that redeploys exactly target's content, or
// an HTTP status and message when it cannot. inspector may be nil.
func Resolve(ctx context.Context, inspector docker.ImageInspector, logger *slog.Logger, target store.DeployAttempt, pin *store.RollbackPin) (ref string, status int, msg string) {
	if target.Status != store.DeployAttemptStatusSucceeded || target.Image == "" {
		return "", http.StatusConflict, "only a succeeded deploy can be rolled back to"
	}
	if ref, safe := deploy.RollbackRef(target, pin); safe {
		return ref, 0, ""
	}
	digest := target.ImageDigest
	if digest == "" {
		return "", http.StatusConflict, "this deploy has no recorded image digest; roll back by image tag with POST /api/v1/apps/{name}/deploys instead"
	}
	if inspector == nil {
		return "", http.StatusNotImplemented, "cannot verify the local image on this control plane"
	}
	id, err := inspector.InspectImageID(ctx, target.Image)
	if err != nil {
		logger.Error("rollback: inspect image failed", slog.String("error", err.Error()), slog.String("image", target.Image))
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
