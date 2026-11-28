package build

import (
	"context"
	"errors"
	"strings"

	"github.com/moby/moby/api/types"
)

// ErrBuildEngineUnsupported means the container engine cannot host a source
// build. The text carries "no buildkit" so failure classification maps it to
// build_engine_unsupported.
var ErrBuildEngineUnsupported = errors.New("build: this engine has no embedded BuildKit (no BuildKit endpoint on its socket), so source builds (Dockerfile, Railpack) cannot run here. " +
	"Set APP_BUILDKIT_IMAGE to a sha256 pinned rootless BuildKit image to use a managed builder, deploy a prebuilt image, or run the build on a Docker node. " +
	"On Ubuntu 24.04, rootless BuildKit can fail to start when kernel.apparmor_restrict_unprivileged_userns=1")

// engineVersioner is the slice of the Docker client the Podman check needs.
type engineVersioner interface {
	ServerVersion(ctx context.Context) (types.Version, error)
}

// checkEngine fails fast on Podman instead of dialing BuildKit and surfacing
// a raw gRPC h2c error. Lookup errors pass through: the solve reports them.
func (c *Client) checkEngine(ctx context.Context) error {
	// A managed rootless builder was already vetted for Podman in NewClient.
	if c.managed {
		return nil
	}
	ev := c.engine
	if ev == nil {
		if c.docker == nil {
			return nil
		}
		ev = c.docker
	}
	if isPodman(ctx, ev) {
		return ErrBuildEngineUnsupported
	}
	return nil
}

// isPodman reports whether the engine identifies as Podman. Lookup errors read as false.
func isPodman(ctx context.Context, ev engineVersioner) bool {
	v, err := ev.ServerVersion(ctx)
	if err != nil {
		return false
	}
	if strings.Contains(strings.ToLower(v.Platform.Name), "podman") {
		return true
	}
	for _, comp := range v.Components {
		if strings.Contains(strings.ToLower(comp.Name), "podman") {
			return true
		}
	}
	return false
}
