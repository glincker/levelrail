package docker

import (
	"context"
	"fmt"
	"strings"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
)

const (
	// envPodmanStopTimeout bounds the graceful stop before a Podman removal.
	envPodmanStopTimeout     = "APP_PODMAN_STOP_TIMEOUT"
	defaultPodmanStopTimeout = 10 * time.Second
)

// Label is the engine's display name, for doctor and status rows.
func (k RuntimeKind) Label() string {
	switch k {
	case RuntimeKindPodman:
		return "Podman"
	case RuntimeKindDocker:
		return "Docker"
	case RuntimeKindDockerDesktop:
		return "Docker Desktop"
	case RuntimeKindColima:
		return "Colima"
	case RuntimeKindOrbStack:
		return "OrbStack"
	case RuntimeKindRancherDesktop:
		return "Rancher Desktop"
	}
	return "Container engine"
}

// isPodman answers from the socket path, then once from the engine itself
// so Podman behind a docker.sock path is still recognised.
func (c *Client) isPodman(ctx context.Context) bool {
	if c.runtime.Kind == RuntimeKindPodman {
		return true
	}
	c.identMu.Lock()
	defer c.identMu.Unlock()
	if !c.identKnown {
		id, err := c.EngineIdentity(ctx)
		if err != nil {
			return false
		}
		c.identPodman = RefineKind(c.runtime.Kind, id) == RuntimeKindPodman
		c.identKnown = true
	}
	return c.identPodman
}

// removePodman stops the container before removing it: the compat API's
// force-remove can leave a running container published after a restart.
func (c *Client) removePodman(ctx context.Context, id string) error {
	seconds := int(envDuration(envPodmanStopTimeout, defaultPodmanStopTimeout).Seconds())
	err := c.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &seconds})
	if err != nil && !cerrdefs.IsNotFound(err) && !cerrdefs.IsNotModified(err) && !strings.Contains(err.Error(), "not running") {
		return fmt.Errorf("docker: stop container %s before removal: %w", id, err)
	}
	if err := c.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: true}); err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		if strings.Contains(err.Error(), "already in progress") && c.waitContainerGone(ctx, id) {
			return nil
		}
		return fmt.Errorf("docker: remove container %s: %w", id, err)
	}
	return nil
}
