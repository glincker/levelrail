package docker

import (
	"context"
	"fmt"
)

// PauseContainer freezes every process in a container (cgroup freezer) so
// its files stop changing without a restart.
func (c *Client) PauseContainer(ctx context.Context, id string) error {
	if err := c.cli.ContainerPause(ctx, id); err != nil {
		return fmt.Errorf("docker: pause container %q: %w", id, err)
	}
	return nil
}

// UnpauseContainer resumes a paused container.
func (c *Client) UnpauseContainer(ctx context.Context, id string) error {
	if err := c.cli.ContainerUnpause(ctx, id); err != nil {
		return fmt.Errorf("docker: unpause container %q: %w", id, err)
	}
	return nil
}
