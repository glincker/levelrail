package build

import (
	"context"
	"fmt"

	dockerclient "github.com/docker/docker/client"
	dockerbuildkit "github.com/docker/docker/client/buildkit"
	bkclient "github.com/moby/buildkit/client"
)

// Client drives builds against the BuildKit instance embedded in a local
// Docker Engine daemon.
//
// Docker Desktop (and any dockerd using the default "docker" buildx
// driver, as opposed to the "docker-container" driver) does not expose a
// bare buildkitd gRPC endpoint of its own. BuildKit runs inside dockerd
// and is only reachable by hijacking the daemon's HTTP connection at the
// /grpc endpoint, exactly the way `docker buildx build` reaches it when
// using the default driver. That is what dockerbuildkit.ClientOpts wires
// up below, using the same *dockerclient.Client already used to talk to
// the Engine API elsewhere in this codebase (see internal/docker).
//
// See docs-local/research/buildkit-spike.md for the connection methods
// that were tried and why this one is the one that actually works in this
// environment, plus what Phase 1's full integration needs to add.
type Client struct {
	bk     *bkclient.Client
	docker *dockerclient.Client
}

// NewClient builds a Client that reaches BuildKit through docker's
// hijacked connection, and reuses the same Docker Engine API client to
// load the finished image back into the local image store.
//
// docker must already be a working *dockerclient.Client (e.g. from
// dockerclient.NewClientWithOpts(dockerclient.FromEnv,
// dockerclient.WithAPIVersionNegotiation())). NewClient does not create
// one itself so callers control the daemon connection lifecycle.
func NewClient(ctx context.Context, docker *dockerclient.Client) (*Client, error) {
	if docker == nil {
		return nil, fmt.Errorf("build: new client: docker client is required")
	}

	bk, err := bkclient.New(ctx, "", dockerbuildkit.ClientOpts(docker)...)
	if err != nil {
		return nil, fmt.Errorf("build: connect to buildkit via docker daemon: %w", err)
	}

	return &Client{bk: bk, docker: docker}, nil
}

// Close releases the underlying BuildKit connection. It does not close
// the Docker Engine API client passed to NewClient, since the caller owns
// that client's lifecycle.
func (c *Client) Close() error {
	if err := c.bk.Close(); err != nil {
		return fmt.Errorf("build: close buildkit client: %w", err)
	}
	return nil
}
