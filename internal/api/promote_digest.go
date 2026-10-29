package api

import (
	"context"
	"log/slog"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// promotionPinnedImage pins source's image to the digest its last succeeded
// deploy recorded, so an approval applies the content the requester saw even
// if the tag moves meanwhile. Without a registry digest it keeps the tag.
func (rt *Router) promotionPinnedImage(ctx context.Context, source store.DesiredService) string {
	ref := promoteImageRef(source)
	if docker.ImageDigestOf(ref) != "" || rt.deployAttempts == nil {
		return ref
	}
	attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, source.Name)
	if err != nil {
		rt.logger.Warn("api: promote: load source deploys for digest pin failed", slog.String("error", err.Error()), slog.String("name", source.Name))
		return ref
	}
	for _, a := range attempts {
		if a.Status != store.DeployAttemptStatusSucceeded || a.ImageDigest == "" || docker.UnpinImageRef(a.Image) != ref {
			continue
		}
		if a.DigestReason == store.DigestReasonResolved || a.DigestReason == store.DigestReasonPinned {
			return docker.PinImageRef(ref, a.ImageDigest)
		}
		return ref
	}
	return ref
}
