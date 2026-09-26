package deploy

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ImageResolution is what a deploy decided an image reference means.
type ImageResolution struct {
	// Image is the reference to write as desired state: pinned to a
	// registry digest whenever one is known.
	Image string
	// Digest is the content identity recorded in deploy history.
	Digest string
	// LocalID is a local image ID, set only when no registry digest exists.
	LocalID string
	// Reason is one of the store.DigestReason* constants.
	Reason string
	// Note carries the registry error behind a fallback, for the operator.
	Note string
}

// RegistryAuthSource resolves a registry credential ID into pull auth.
type RegistryAuthSource interface {
	RegistryAuth(ctx context.Context, credentialID string) (*docker.RegistryAuth, error)
}

// WithImageResolver makes image deploys resolve floating tags to digests.
// Without one, image references are written exactly as given.
func WithImageResolver(r docker.ImageResolver) Option {
	return func(p *Pipeline) { p.resolver = r }
}

// WithImageInspector lets a build record the local image ID it produced.
func WithImageInspector(i docker.ImageInspector) Option {
	return func(p *Pipeline) { p.inspector = i }
}

// WithRegistryAuthSource lets digest resolution authenticate against a
// private registry.
func WithRegistryAuthSource(s RegistryAuthSource) Option {
	return func(p *Pipeline) { p.registryAuth = s }
}

// ResolveImage turns image into content. A nil resolver keeps the image as
// given. requireFresh fails instead of falling back to a cached image.
func ResolveImage(ctx context.Context, r docker.ImageResolver, image string, auth *docker.RegistryAuth, requireFresh bool) (ImageResolution, error) {
	if d := docker.ImageDigestOf(image); d != "" {
		return ImageResolution{Image: image, Digest: d, Reason: store.DigestReasonPinned}, nil
	}
	if r == nil {
		return ImageResolution{Image: image}, nil
	}
	res, err := r.ResolveImage(ctx, image, auth, requireFresh)
	if err != nil {
		return ImageResolution{}, fmt.Errorf("resolve image %q: %w", image, err)
	}
	out := ImageResolution{Image: res.Ref, Digest: res.Digest, LocalID: res.LocalID}
	if out.Digest == "" {
		out.Digest = res.LocalID
	}
	if res.RegistryErr != nil {
		out.Note = res.RegistryErr.Error()
	}
	switch res.Source {
	case docker.ImageSourcePinned:
		out.Reason = store.DigestReasonPinned
	case docker.ImageSourceRegistry:
		out.Reason = store.DigestReasonResolved
	case docker.ImageSourceCache:
		out.Reason = store.DigestReasonPullFailedUsingCached
	default:
		out.Reason = store.DigestReasonUnresolved
	}
	return out, nil
}

// apply writes res onto desired: the pinned image, plus the local ID when
// that is all that identifies the content.
func (res ImageResolution) apply(desired *store.DesiredService) {
	desired.Image = res.Image
	desired.ImageID, desired.ImageIDRef = "", ""
	if res.LocalID != "" && docker.ImageDigestOf(res.Image) == "" {
		desired.ImageID, desired.ImageIDRef = res.LocalID, res.Image
	}
}

func (p *Pipeline) resolveImage(ctx context.Context, image, credentialID string, requireFresh bool) (ImageResolution, error) {
	var auth *docker.RegistryAuth
	if credentialID != "" && p.registryAuth != nil && p.resolver != nil {
		a, err := p.registryAuth.RegistryAuth(ctx, credentialID)
		if err != nil {
			return ImageResolution{}, fmt.Errorf("registry credential: %w", err)
		}
		auth = a
	}
	return ResolveImage(ctx, p.resolver, image, auth, requireFresh)
}

// buildResolution identifies a freshly built tag by the local image ID the
// daemon reports, falling back to BuildKit's exporter digest for display.
func (p *Pipeline) buildResolution(ctx context.Context, tag string, exporter map[string]string) ImageResolution {
	res := ImageResolution{Image: tag, Reason: store.DigestReasonLocalBuild}
	if p.inspector != nil {
		if id, err := p.inspector.InspectImageID(ctx, tag); err == nil && id != "" {
			res.Digest, res.LocalID = id, id
			return res
		}
	}
	res.Digest = exporter["containerimage.digest"]
	return res
}
