package docker

import (
	"context"
	"fmt"
	"strings"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/distribution/reference"
	"github.com/docker/docker/api/types/registry"
)

// ImageSource says where ResolveImage got an image's content identity.
type ImageSource string

const (
	// ImageSourcePinned means the reference already carried a digest.
	ImageSourcePinned ImageSource = "pinned"
	// ImageSourceRegistry means the registry answered for the tag.
	ImageSourceRegistry ImageSource = "registry"
	// ImageSourceCache means the registry failed and the local image was used.
	ImageSourceCache ImageSource = "cache"
	// ImageSourceNone means neither the registry nor the local cache knew the tag.
	ImageSourceNone ImageSource = "none"
)

// ResolvedImage is a floating reference turned into content.
type ResolvedImage struct {
	// Ref is what to create containers from: the input pinned to Digest
	// when a registry digest is known, otherwise the input unchanged.
	Ref string
	// Digest is the registry digest (a manifest list digest for a
	// multi-arch image), empty when only a local image ID is known.
	Digest string
	// LocalID is the local image ID when the image was found only locally.
	LocalID string
	Source  ImageSource
	// RegistryErr is why the registry lookup failed, for Cache and None.
	RegistryErr error
}

// ImageResolver resolves a tag to content at deploy time. requireFresh
// turns a registry failure into an error instead of a cache fallback.
type ImageResolver interface {
	ResolveImage(ctx context.Context, ref string, auth *RegistryAuth, requireFresh bool) (ResolvedImage, error)
}

// ImageInspector reports the local image ID a reference resolves to on the
// node, empty when the image is not present there.
type ImageInspector interface {
	InspectImageID(ctx context.Context, ref string) (string, error)
}

// PinImageRef appends digest to ref unless ref already carries one.
func PinImageRef(ref, digest string) string {
	if digest == "" || strings.Contains(ref, "@") {
		return ref
	}
	return ref + "@" + digest
}

// UnpinImageRef strips a trailing "@digest" so a pinned reference can be
// re-resolved (deploy --pull) or shown as its tag.
func UnpinImageRef(ref string) string {
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[:i]
	}
	return ref
}

// ImageDigestOf returns the digest a pinned reference carries, or "".
func ImageDigestOf(ref string) string {
	if i := strings.LastIndex(ref, "@"); i > 0 {
		return ref[i+1:]
	}
	return ""
}

// ResolveImage implements ImageResolver.
func (c *Client) ResolveImage(ctx context.Context, ref string, auth *RegistryAuth, requireFresh bool) (ResolvedImage, error) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return ResolvedImage{}, fmt.Errorf("docker: resolve image %q: %w", ref, err)
	}
	if canonical, ok := named.(reference.Canonical); ok {
		return ResolvedImage{Ref: ref, Digest: canonical.Digest().String(), Source: ImageSourcePinned}, nil
	}

	encoded := ""
	if auth != nil {
		encoded, err = registry.EncodeAuthConfig(registry.AuthConfig{Username: auth.Username, Password: auth.Password})
		if err != nil {
			return ResolvedImage{}, fmt.Errorf("docker: resolve image %q: encode auth: %w", ref, err)
		}
	}
	info, regErr := c.cli.DistributionInspect(ctx, ref, encoded)
	if regErr == nil && info.Descriptor.Digest != "" {
		d := info.Descriptor.Digest.String()
		return ResolvedImage{Ref: PinImageRef(ref, d), Digest: d, Source: ImageSourceRegistry}, nil
	}
	if regErr == nil {
		regErr = fmt.Errorf("registry returned no digest")
	}
	if requireFresh {
		return ResolvedImage{}, fmt.Errorf("docker: resolve image %q from registry: %w", ref, regErr)
	}

	local, err := c.cli.ImageInspect(ctx, ref)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return ResolvedImage{Ref: ref, Source: ImageSourceNone, RegistryErr: regErr}, nil
		}
		return ResolvedImage{}, fmt.Errorf("docker: resolve image %q: inspect local: %w", ref, err)
	}
	if d := repoDigestFor(named, local.RepoDigests); d != "" {
		return ResolvedImage{Ref: PinImageRef(ref, d), Digest: d, Source: ImageSourceCache, RegistryErr: regErr}, nil
	}
	return ResolvedImage{Ref: ref, LocalID: local.ID, Source: ImageSourceCache, RegistryErr: regErr}, nil
}

// repoDigestFor picks the digest of the repo digest entry matching named.
func repoDigestFor(named reference.Named, repoDigests []string) string {
	for _, rd := range repoDigests {
		parsed, err := reference.ParseNormalizedNamed(rd)
		if err != nil {
			continue
		}
		canonical, ok := parsed.(reference.Canonical)
		if ok && parsed.Name() == named.Name() {
			return canonical.Digest().String()
		}
	}
	return ""
}

// InspectImageID implements ImageInspector.
func (c *Client) InspectImageID(ctx context.Context, ref string) (string, error) {
	resp, err := c.cli.ImageInspect(ctx, ref)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return "", nil
		}
		return "", fmt.Errorf("docker: inspect image %q: %w", ref, err)
	}
	return resp.ID, nil
}
