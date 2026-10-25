package build

import (
	"context"
	"errors"
	"strings"

	"github.com/docker/docker/api/types"
)

// ErrBuildEngineUnsupported means the container engine cannot host a source
// build. The text carries "no buildkit" so failure classification maps it to
// build_engine_unsupported.
var ErrBuildEngineUnsupported = errors.New("build: source builds (Dockerfile, Railpack) are not supported on Podman: no BuildKit is reachable through its socket. " +
	"Podman source builds are planned via a managed buildkitd. Image deploys work on Podman today, so deploy a prebuilt image or run the build on a Docker node")

// engineVersioner is the slice of the Docker client the Podman check needs.
type engineVersioner interface {
	ServerVersion(ctx context.Context) (types.Version, error)
}

// checkEngine fails fast on Podman instead of dialing BuildKit and surfacing
// a raw gRPC h2c error. Lookup errors pass through: the solve reports them.
func (c *Client) checkEngine(ctx context.Context) error {
	ev := c.engine
	if ev == nil {
		if c.docker == nil {
			return nil
		}
		ev = c.docker
	}
	v, err := ev.ServerVersion(ctx)
	if err != nil {
		return nil
	}
	if strings.Contains(strings.ToLower(v.Platform.Name), "podman") {
		return ErrBuildEngineUnsupported
	}
	for _, comp := range v.Components {
		if strings.Contains(strings.ToLower(comp.Name), "podman") {
			return ErrBuildEngineUnsupported
		}
	}
	return nil
}
