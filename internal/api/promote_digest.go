package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

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

// verifyPromotionDigest rejects an approval whose pinned digest the registry
// no longer knows, so it fails here with advice instead of later in reconcile.
// An unreachable or unsupported registry check never blocks the approval.
func (rt *Router) verifyPromotionDigest(ctx context.Context, image string) error {
	if docker.ImageDigestOf(image) == "" {
		return nil
	}
	verifier, ok := rt.imageResolver.(docker.DigestVerifier)
	if !ok {
		return nil
	}
	if d, err := time.ParseDuration(os.Getenv("APP_APPROVE_DIGEST_CHECK_TIMEOUT")); err == nil && d > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d)
		defer cancel()
	}
	exists, err := verifier.ImageDigestExists(ctx, image)
	if err != nil {
		rt.logger.Warn("api: promote: digest check inconclusive", slog.String("error", err.Error()), slog.String("image", image))
		return nil
	}
	if !exists {
		return &deployApprovalDecisionError{http.StatusGone, fmt.Sprintf("image %s is no longer in its registry; the approval is still pending, reject it and request the promotion again", image)}
	}
	return nil
}
