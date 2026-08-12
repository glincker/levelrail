package docker

import (
	"context"
	"fmt"
	"io"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	dockerclient "github.com/docker/docker/client"
)

// Client is the real Runtime implementation, talking to the Docker Engine
// API over the local socket. It never shells out to the `docker` CLI
// (CLAUDE.md 4.3).
type Client struct {
	cli *dockerclient.Client
}

// NewClient builds a Client from the standard Docker environment
// (DOCKER_HOST, DOCKER_CERT_PATH, etc.), negotiating API version against
// whatever daemon is actually running rather than pinning one.
func NewClient() (*Client, error) {
	cli, err := dockerclient.NewClientWithOpts(
		dockerclient.FromEnv,
		dockerclient.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return nil, fmt.Errorf("docker: new client: %w", err)
	}
	return &Client{cli: cli}, nil
}

// Close releases the underlying HTTP transport.
func (c *Client) Close() error {
	return c.cli.Close()
}

// InspectByName implements Runtime.
func (c *Client) InspectByName(ctx context.Context, name string) (*ContainerState, error) {
	f := filters.NewArgs()
	f.Add("name", "^/"+name+"$") // anchored: Docker's name filter is a substring match otherwise
	summaries, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers named %q: %w", name, err)
	}
	if len(summaries) == 0 {
		return nil, nil
	}
	s := summaries[0]
	return &ContainerState{
		ID:      s.ID,
		Name:    name,
		Image:   s.Image,
		Running: s.State == "running",
	}, nil
}

// Create implements Runtime.
func (c *Client) Create(ctx context.Context, spec ContainerSpec) (string, error) {
	if err := c.ensureImage(ctx, spec.Image); err != nil {
		return "", err
	}

	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{Image: spec.Image},
		&container.HostConfig{},
		nil, nil,
		spec.Name,
	)
	if err != nil {
		return "", fmt.Errorf("docker: create container %q: %w", spec.Name, err)
	}
	return resp.ID, nil
}

// Start implements Runtime.
func (c *Client) Start(ctx context.Context, id string) error {
	if err := c.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("docker: start container %s: %w", id, err)
	}
	return nil
}

// ensureImage pulls ref if it isn't already present locally. Reconcile
// calls happen often (event-driven plus periodic resync); pulling
// unconditionally on every call would be slow and noisy, so this checks
// first and only pulls on a genuine local miss.
func (c *Client) ensureImage(ctx context.Context, ref string) error {
	_, _, err := c.cli.ImageInspectWithRaw(ctx, ref)
	if err == nil {
		return nil
	}
	if !dockerclient.IsErrNotFound(err) {
		return fmt.Errorf("docker: inspect image %q: %w", ref, err)
	}

	rc, err := c.cli.ImagePull(ctx, ref, image.PullOptions{})
	if err != nil {
		return fmt.Errorf("docker: pull image %q: %w", ref, err)
	}
	defer func() {
		_ = rc.Close() // read side, fully drained by io.Copy below; nothing actionable on a close error here
	}()
	if _, err := io.Copy(io.Discard, rc); err != nil {
		return fmt.Errorf("docker: pull image %q: reading progress stream: %w", ref, err)
	}
	return nil
}

// Events implements Runtime.
func (c *Client) Events(ctx context.Context) (<-chan Event, <-chan error) {
	f := filters.NewArgs()
	f.Add("type", string(events.ContainerEventType))

	rawEvents, rawErrs := c.cli.Events(ctx, events.ListOptions{Filters: f})

	out := make(chan Event)
	outErr := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(outErr)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-rawErrs:
				if !ok {
					return
				}
				if err != nil {
					outErr <- err
				}
				return
			case msg, ok := <-rawEvents:
				if !ok {
					return
				}
				action := mapAction(msg.Action)
				if action == "" {
					continue // not a lifecycle action a controller needs
				}
				select {
				case out <- Event{
					Action:        action,
					ContainerName: msg.Actor.Attributes["name"],
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, outErr
}

func mapAction(a events.Action) EventAction {
	switch string(a) {
	case "start":
		return EventStart
	case "die":
		return EventDie
	case "stop":
		return EventStop
	default:
		return ""
	}
}
