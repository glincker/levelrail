package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/deploytrace"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ResolveImageTraced is ResolveImage recording the digest lookup and the
// registry auth check on tr. previous is the image the service ran before
// (to say whether the tag moved); empty when unknown.
func ResolveImageTraced(ctx context.Context, tr *deploytrace.Tracer, r docker.ImageResolver, image string, auth *docker.RegistryAuth, requireFresh bool, previous string) (ImageResolution, error) {
	host := docker.RegistryHost(image)
	span := tr.Begin(ctx, deploytrace.StepResolveDigest, fmt.Sprintf("resolving %s on %s", image, host))
	res, err := ResolveImage(ctx, r, image, auth, requireFresh)
	traceRegistryAuth(ctx, tr, auth, host, err)
	if err != nil {
		span.Fail(ctx, "DigestResolveFailed", err)
		return res, err
	}
	detail := fmt.Sprintf("registry %s, digest %s, %s", host, orNone(res.Digest), tagMovement(previous, res))
	switch res.Reason {
	case store.DigestReasonPullFailedUsingCached:
		span.End(ctx, store.TraceStatusWarning, res.Reason, "registry unreachable, using the cached local image: "+res.Note)
	case store.DigestReasonLocalImage:
		span.End(ctx, store.TraceStatusSkipped, res.Reason, "image exists only locally, no registry digest to resolve")
	case store.DigestReasonUnresolved:
		span.End(ctx, store.TraceStatusWarning, res.Reason, "no digest known, the node pulls the tag as given")
	case store.DigestReasonPinned:
		span.End(ctx, store.TraceStatusSkipped, res.Reason, "reference already pinned: "+orNone(res.Digest))
	default:
		span.Done(ctx, detail)
	}
	return res, nil
}

func traceRegistryAuth(ctx context.Context, tr *deploytrace.Tracer, auth *docker.RegistryAuth, host string, resolveErr error) {
	switch {
	case auth == nil:
		tr.Note(ctx, deploytrace.StepRegistryAuth, store.TraceStatusSkipped, "NoCredential", "no registry credential, anonymous access to "+host)
	case resolveErr != nil && authDenied(resolveErr):
		tr.Note(ctx, deploytrace.StepRegistryAuth, store.TraceStatusFailed, "RegistryAuthDenied", resolveErr.Error())
	default:
		tr.Note(ctx, deploytrace.StepRegistryAuth, store.TraceStatusDone, "", fmt.Sprintf("authenticated to %s as %s", host, auth.Username))
	}
}

func authDenied(err error) bool {
	m := strings.ToLower(err.Error())
	return strings.Contains(m, "unauthorized") || strings.Contains(m, "denied") || strings.Contains(m, "authentication required")
}

func tagMovement(previous string, res ImageResolution) string {
	prev := docker.ImageDigestOf(previous)
	switch prev {
	case "":
		return "no previous digest to compare"
	case res.Digest:
		return "unchanged since the last deploy"
	}
	return "changed from " + prev
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}
