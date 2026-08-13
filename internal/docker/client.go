package docker

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
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
	return toContainerState(summaries[0]), nil
}

// ListByPrefix implements Runtime.
func (c *Client) ListByPrefix(ctx context.Context, prefix string) ([]ContainerState, error) {
	f := filters.NewArgs()
	f.Add("name", "^/"+prefix) // no trailing $: prefix match, not exact
	summaries, err := c.cli.ContainerList(ctx, container.ListOptions{All: true, Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list containers with prefix %q: %w", prefix, err)
	}
	if len(summaries) == 0 {
		return nil, nil
	}
	out := make([]ContainerState, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, *toContainerState(s))
	}
	return out, nil
}

func toContainerState(s dockertypes.Container) *ContainerState {
	name := ""
	if len(s.Names) > 0 {
		name = strings.TrimPrefix(s.Names[0], "/")
	}
	return &ContainerState{
		ID:      s.ID,
		Name:    name,
		Image:   s.Image,
		Running: s.State == "running",
		Ports:   observedPorts(s.Ports),
	}
}

func observedPorts(ports []dockertypes.Port) []PortBinding {
	// Docker reports one entry per (IP, port) combination, so a port
	// published on all interfaces typically appears twice, once for the
	// IPv4 wildcard and once for IPv6. PortBinding doesn't carry the
	// bind IP, so from a caller's view those are the same binding;
	// dedupe on the fields that are actually kept.
	seen := make(map[PortBinding]bool, len(ports))
	var out []PortBinding
	for _, p := range ports {
		if p.PublicPort == 0 {
			continue // exposed but not published to a host port, nothing for a caller to route to
		}
		binding := PortBinding{
			ContainerPort: int(p.PrivatePort),
			HostPort:      int(p.PublicPort),
			Protocol:      p.Type,
		}
		if seen[binding] {
			continue
		}
		seen[binding] = true
		out = append(out, binding)
	}
	return out
}

// Create implements Runtime.
func (c *Client) Create(ctx context.Context, spec ContainerSpec) (string, error) {
	if err := c.ensureImage(ctx, spec.Image); err != nil {
		return "", err
	}

	exposedPorts, portBindings, err := toDockerPorts(spec.Ports)
	if err != nil {
		return "", fmt.Errorf("docker: create container %q: %w", spec.Name, err)
	}

	hostConfig := &container.HostConfig{
		PortBindings: portBindings,
		// Explicitly "no": the reconciler, not Docker, decides whether a
		// dead container comes back. See ContainerSpec's doc comment.
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
		Mounts:        toDockerMounts(spec.Volumes),
	}
	if spec.Resources != nil {
		hostConfig.Resources = container.Resources{
			Memory:   spec.Resources.MemoryBytes,
			NanoCPUs: spec.Resources.NanoCPUs,
		}
	}

	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image:        spec.Image,
			ExposedPorts: exposedPorts,
			Env:          toDockerEnv(spec.Env),
		},
		hostConfig,
		nil, nil,
		spec.Name,
	)
	if err != nil {
		return "", fmt.Errorf("docker: create container %q: %w", spec.Name, err)
	}
	return resp.ID, nil
}

func toDockerPorts(ports []PortBinding) (nat.PortSet, nat.PortMap, error) {
	if len(ports) == 0 {
		return nil, nil, nil
	}

	exposed := make(nat.PortSet, len(ports))
	bindings := make(nat.PortMap, len(ports))

	for _, p := range ports {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		natPort, err := nat.NewPort(proto, strconv.Itoa(p.ContainerPort))
		if err != nil {
			return nil, nil, fmt.Errorf("port %d/%s: %w", p.ContainerPort, proto, err)
		}

		exposed[natPort] = struct{}{}

		hostPort := ""
		if p.HostPort != 0 {
			hostPort = strconv.Itoa(p.HostPort)
		}
		bindings[natPort] = append(bindings[natPort], nat.PortBinding{HostPort: hostPort})
	}

	return exposed, bindings, nil
}

func toDockerMounts(volumes []VolumeMount) []mount.Mount {
	if len(volumes) == 0 {
		return nil
	}
	out := make([]mount.Mount, 0, len(volumes))
	for _, v := range volumes {
		out = append(out, mount.Mount{
			Type:   mount.TypeVolume,
			Source: v.Name,
			Target: v.ContainerPath,
		})
	}
	return out
}

func toDockerEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, k+"="+v)
	}
	sort.Strings(out) // deterministic order, mainly so tests aren't flaky
	return out
}

// Start implements Runtime.
func (c *Client) Start(ctx context.Context, id string) error {
	if err := c.cli.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		return fmt.Errorf("docker: start container %s: %w", id, err)
	}
	return nil
}

// Stop implements Runtime.
func (c *Client) Stop(ctx context.Context, id string, timeout time.Duration) error {
	opts := container.StopOptions{}
	if timeout >= 0 {
		seconds := int(timeout.Seconds())
		opts.Timeout = &seconds
	} else {
		negativeOne := -1
		opts.Timeout = &negativeOne // Engine API convention: -1 means wait indefinitely
	}
	if err := c.cli.ContainerStop(ctx, id, opts); err != nil {
		return fmt.Errorf("docker: stop container %s: %w", id, err)
	}
	return nil
}

// Remove implements Runtime.
func (c *Client) Remove(ctx context.Context, id string, force bool) error {
	if err := c.cli.ContainerRemove(ctx, id, container.RemoveOptions{Force: force}); err != nil {
		return fmt.Errorf("docker: remove container %s: %w", id, err)
	}
	return nil
}

// EnsureVolume implements Runtime. Docker's VolumeCreate is itself
// idempotent by name (creating a volume that already exists returns the
// existing volume, not an error), so this is a thin wrapper, not a
// check-then-create with its own race window.
func (c *Client) EnsureVolume(ctx context.Context, name string) error {
	if _, err := c.cli.VolumeCreate(ctx, volume.CreateOptions{Name: name}); err != nil {
		return fmt.Errorf("docker: ensure volume %q: %w", name, err)
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

// ListImages implements Runtime.
func (c *Client) ListImages(ctx context.Context, repo string) ([]ImageInfo, error) {
	f := filters.NewArgs()
	f.Add("reference", repo)

	summaries, err := c.cli.ImageList(ctx, image.ListOptions{Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list images for %q: %w", repo, err)
	}

	var out []ImageInfo
	for _, s := range summaries {
		createdAt := time.Unix(s.Created, 0).UTC()
		for _, tag := range s.RepoTags {
			out = append(out, ImageInfo{Tag: tag, CreatedAt: createdAt})
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })

	return out, nil
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
					Time:          eventTime(msg),
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return out, outErr
}

// eventTime extracts msg's timestamp, preferring TimeNano (full
// nanosecond-precision Unix time, not a fractional remainder of Time)
// when the daemon sets it, falling back to the whole-second Time field
// for older daemons or event sources that only populate that one.
// Without this, every Event's Time field silently stayed the zero
// value: nothing broke loudly (Action/ContainerName, the only fields
// every other consumer of Events reads, were always populated
// correctly), but any consumer keyed on Time, like
// internal/alerting.RestartTracker's restart-window counting, would
// silently never count anything against a real time window.
func eventTime(msg events.Message) time.Time {
	if msg.TimeNano != 0 {
		return time.Unix(0, msg.TimeNano)
	}
	if msg.Time != 0 {
		return time.Unix(msg.Time, 0)
	}
	return time.Time{}
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
