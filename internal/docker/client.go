package docker

import (
	"bytes"
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
	dockernetwork "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
)

// Client is the real Runtime implementation, talking to the Docker Engine
// API over the local socket. It never shells out to the `docker` CLI,
// per the project's Docker Engine API only rule.
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

// Ping is a thin liveness check against the Docker daemon, wrapping the
// underlying SDK client's own Ping. Deliberately a method on the
// concrete *Client, not a new Runtime method: Runtime has 6+ fake
// implementations across internal/reconcile's controllers and
// internal/agent's test files, plus the real client here and the
// agent-side remote transport, so adding a method there would ripple
// into all of them for the sake of one read-only health signal that
// GET /api/v1/system/status needs. internal/api.DockerPinger is the
// narrow interface that actually consumes this, satisfied structurally
// by *Client alone.
func (c *Client) Ping(ctx context.Context) error {
	if _, err := c.cli.Ping(ctx); err != nil {
		return fmt.Errorf("docker: ping: %w", err)
	}
	return nil
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

	hostConfig := buildHostConfig(spec, portBindings)

	resp, err := c.cli.ContainerCreate(ctx,
		&container.Config{
			Image:        spec.Image,
			ExposedPorts: exposedPorts,
			Env:          toDockerEnv(spec.Env),
			Labels:       spec.Labels,
		},
		hostConfig,
		nil, nil,
		spec.Name,
	)
	if err != nil {
		return "", fmt.Errorf("docker: create container %q: %w", spec.Name, err)
	}

	// spec.Network is attached via a separate NetworkConnect call, not
	// ContainerCreate's own NetworkingConfig argument: passing endpoint
	// config (in particular a non-empty Aliases list) directly to
	// ContainerCreate reproducibly hangs against this project's own
	// development daemon (Docker Desktop for Mac), waiting on the
	// create response indefinitely, while the identical attachment via
	// `docker create --network --network-alias` on the CLI (a
	// create-then-connect two-step under the hood) returns instantly.
	// Connecting after create sidesteps whatever that daemon-side
	// interaction is, and is the more broadly-compatible pattern the
	// Docker Go SDK's own issue tracker has long recommended over
	// endpoint config at creation time for exactly this class of
	// problem.
	if spec.Network != nil && spec.Network.Name != "" {
		endpoint := &dockernetwork.EndpointSettings{}
		if spec.Network.Alias != "" {
			endpoint.Aliases = []string{spec.Network.Alias}
		}
		if err := c.cli.NetworkConnect(ctx, spec.Network.Name, resp.ID, endpoint); err != nil {
			return "", fmt.Errorf("docker: connect container %q to network %q: %w", spec.Name, spec.Network.Name, err)
		}
	}

	return resp.ID, nil
}

// EnsureNetwork implements Runtime.
func (c *Client) EnsureNetwork(ctx context.Context, name string) (string, error) {
	inspect, err := c.cli.NetworkInspect(ctx, name, dockernetwork.InspectOptions{})
	if err == nil {
		return inspect.ID, nil
	}
	if !dockerclient.IsErrNotFound(err) {
		return "", fmt.Errorf("docker: inspect network %q: %w", name, err)
	}

	resp, err := c.cli.NetworkCreate(ctx, name, dockernetwork.CreateOptions{Driver: "bridge"})
	if err != nil {
		return "", fmt.Errorf("docker: create network %q: %w", name, err)
	}
	return resp.ID, nil
}

// RemoveNetwork implements Runtime.
func (c *Client) RemoveNetwork(ctx context.Context, name string) error {
	if err := c.cli.NetworkRemove(ctx, name); err != nil {
		if dockerclient.IsErrNotFound(err) {
			return nil
		}
		return fmt.Errorf("docker: remove network %q: %w", name, err)
	}
	return nil
}

// ListNetworksByPrefix implements Runtime.
func (c *Client) ListNetworksByPrefix(ctx context.Context, prefix string) ([]NetworkInfo, error) {
	f := filters.NewArgs()
	f.Add("name", prefix) // Docker's network name filter is a substring match; narrowed by the prefix check below
	networks, err := c.cli.NetworkList(ctx, dockernetwork.ListOptions{Filters: f})
	if err != nil {
		return nil, fmt.Errorf("docker: list networks with prefix %q: %w", prefix, err)
	}

	out := make([]NetworkInfo, 0, len(networks))
	for _, n := range networks {
		if !strings.HasPrefix(n.Name, prefix) {
			continue
		}
		out = append(out, NetworkInfo{ID: n.ID, Name: n.Name})
	}
	return out, nil
}

// buildHostConfig turns spec plus its already-computed port bindings into
// the container.HostConfig Create passes to the Engine API. Factored out
// of Create so its own now-linear body isn't cluttered by this
// construction, and so a future caller (none today) could reuse the same
// spec-to-HostConfig mapping without duplicating it.
func buildHostConfig(spec ContainerSpec, portBindings nat.PortMap) *container.HostConfig {
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
	if len(spec.DNS) > 0 {
		hostConfig.DNS = spec.DNS
	}
	return hostConfig
}

// BridgeGatewayIP returns the gateway IP of Docker's default "bridge"
// network: the address from which a container attached to that network
// (the default whenever Create's NetworkingConfig is nil, which is every
// container this Client creates) can reach the host. Used by
// cmd/levelrail/mesh.go's containerDNSAddr to compute a
// container-reachable address for the mesh DNS server, since no
// WireGuard mesh IP is bound to anything yet.
func (c *Client) BridgeGatewayIP(ctx context.Context) (string, error) {
	inspect, err := c.cli.NetworkInspect(ctx, "bridge", dockernetwork.InspectOptions{})
	if err != nil {
		return "", fmt.Errorf("docker: inspect bridge network: %w", err)
	}
	return gatewayIPFromNetworkInspect(inspect.IPAM.Config)
}

// gatewayIPFromNetworkInspect picks the first non-empty gateway out of
// cfg, split out of BridgeGatewayIP so it can be table-tested against the
// handful of shapes IPAM.Config can actually take (no entries, one entry
// with a gateway, one entry without, several) without a live daemon.
func gatewayIPFromNetworkInspect(cfg []dockernetwork.IPAMConfig) (string, error) {
	if len(cfg) == 0 {
		return "", fmt.Errorf("docker: bridge network has no IPAM config")
	}
	for _, c := range cfg {
		if c.Gateway != "" {
			return c.Gateway, nil
		}
	}
	return "", fmt.Errorf("docker: bridge network IPAM config has no gateway")
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

// execStderrCap bounds how much of a failed exec's stderr Exec captures
// for the error message it builds. Real diagnostic text from the
// commands this method is meant for (a bad flag, a rejected connection,
// an auth failure) is a handful of lines at most; anything beyond this
// cap almost certainly means the command isn't behaving the way Exec's
// contract assumes (stdout carries the real payload, stderr is
// informational only), so capping bounds memory instead of trusting
// every command to stay well-behaved.
const execStderrCap = 64 * 1024

// cappedBuffer is a bytes.Buffer that silently drops bytes past limit
// instead of growing without bound. Used only for the stderr side of
// Exec's output: losing the tail of an overlong error message is
// acceptable, the alternative (an unbounded buffer fed by a command this
// package doesn't control) is not.
type cappedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	remaining := c.limit - c.buf.Len()
	if remaining <= 0 {
		return len(p), nil // over the cap: report a full write so stdcopy doesn't treat this as a failure, just drop the bytes
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	c.buf.Write(p)
	return len(p), nil
}

// Exec implements Runtime. It creates an exec configuration on
// containerID, attaches to it to actually run cmd, and returns a
// ReadCloser streaming cmd's stdout back to the caller.
//
// Docker's exec attach stream, with no tty (never requested here), is
// always multiplexed per pkg/stdcopy's frame format even though this
// call only asks for one logical output: stdout is copied straight
// through to the pipe this method returns, stderr is captured
// separately into a capped buffer purely so a non-zero exit has
// something useful to explain itself with. ContainerExecAttach itself
// has no notion of exit status, only ContainerExecInspect does, so exit
// code is checked only after the stream ends, in the background
// goroutine that drives the copy.
func (c *Client) Exec(ctx context.Context, containerID string, cmd []string) (io.ReadCloser, error) {
	execID, attached, err := c.execCreateAttach(ctx, containerID, cmd, false)
	if err != nil {
		return nil, err
	}

	pr, pw := io.Pipe()
	go c.streamExecOutput(ctx, containerID, execID, cmd, attached, pw)
	return pr, nil
}

// ExecWithInput implements Runtime. It is execCreateAttach plus a stdin
// side: everything Exec does (see Exec's own doc comment for the
// stdout/stderr/exit-code handling, unchanged here, both methods share
// streamExecOutput) plus a second goroutine that copies stdin into the
// exec session's write side and then closes it, the same "close the
// write half once the reader is exhausted" signal a real pipe or
// terminal gives a process reading until EOF.
//
// The stdin-copy goroutine and streamExecOutput's own stdout-copy
// goroutine run concurrently, not sequentially: a command that starts
// producing output before this process has finished writing every byte
// of stdin (true of psql on a large dump, which echoes NOTICE/COPY
// progress lines as it goes) would otherwise deadlock, stdin blocked on
// a full kernel pipe buffer nobody is draining because this method
// hadn't gotten to the read side yet.
func (c *Client) ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	execID, attached, err := c.execCreateAttach(ctx, containerID, cmd, true)
	if err != nil {
		return nil, err
	}

	go func() {
		_, copyErr := io.Copy(attached.Conn, stdin)
		// CloseWrite, not Close: the read side (stdout/stderr, drained by
		// streamExecOutput below) is still in use after this goroutine is
		// done with the write side, and attached itself is only closed
		// once, by streamExecOutput's own deferred attached.Close(). A
		// stdin-side copy failure has nowhere to go but here: the caller
		// finds out about it indirectly, through the exec'd command's own
		// reaction to a truncated stdin (a non-zero exit, surfaced by
		// streamExecOutput the same way every other exec failure already
		// is), not through a second, separate error path this method
		// would otherwise need to invent.
		_ = copyErr
		_ = attached.CloseWrite()
	}()

	pr, pw := io.Pipe()
	go c.streamExecOutput(ctx, containerID, execID, cmd, attached, pw)
	return pr, nil
}

// execCreateAttach is Exec and ExecWithInput's shared setup: create an
// exec configuration on containerID for cmd, then attach to it. Factored
// out so the two methods' only real difference, whether AttachStdin is
// set, is a single boolean parameter rather than two near-identical
// copies of ContainerExecCreate/ContainerExecAttach drifting apart over
// time.
func (c *Client) execCreateAttach(ctx context.Context, containerID string, cmd []string, attachStdin bool) (execID string, attached dockertypes.HijackedResponse, err error) {
	created, err := c.cli.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdin:  attachStdin,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return "", dockertypes.HijackedResponse{}, fmt.Errorf("docker: exec create %v in %s: %w", cmd, containerID, err)
	}

	attached, err = c.cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		return "", dockertypes.HijackedResponse{}, fmt.Errorf("docker: exec attach %v in %s: %w", cmd, containerID, err)
	}
	return created.ID, attached, nil
}

// ExecExitError is the trailing error Exec's and ExecWithInput's
// returned io.ReadCloser produce, once their stream ends, when cmd
// exited non-zero inside the container: see Runtime.Exec's own doc
// comment for why an exit code is folded into the error a later Read
// returns rather than carried as a second out-of-band field. Every
// caller before this type existed (internal/backup's Dumper and
// Restorer) only ever needed "err != nil means the dump/restore
// failed," so ExitCode and Stderr lived only inside the error's
// formatted message, unrecoverable except by parsing text. This type
// keeps that exact message (Error() reproduces streamExecOutput's
// original format string byte for byte, so nothing that already checks
// err != nil or matches on message substrings, e.g. client_live_test.go's
// "exited 7" check, changes behavior) while making ExitCode and Stderr
// recoverable via errors.As, for a caller like internal/api's exec
// endpoint that has to report a real exit code and stderr back to an
// HTTP client instead of only "the command failed."
type ExecExitError struct {
	Cmd       []string
	Container string
	ExitCode  int
	Stderr    string
}

func (e *ExecExitError) Error() string {
	return fmt.Sprintf("docker: exec %v in %s: exited %d: %s", e.Cmd, e.Container, e.ExitCode, e.Stderr)
}

// streamExecOutput drives the copy from attached's multiplexed stream
// into pw (stdout) and a capped stderr buffer, then resolves pw's
// eventual EOF or error based on the exec's real exit code, not merely
// whether the copy itself errored: a command that writes nothing and
// exits 1 (a missing binary, a rejected connection before any output)
// would otherwise look identical to one that succeeded with an empty
// dump, and that distinction is exactly what a backup caller needs to
// tell "empty database" apart from "the dump silently failed."
func (c *Client) streamExecOutput(ctx context.Context, containerID, execID string, cmd []string, attached dockertypes.HijackedResponse, pw *io.PipeWriter) {
	defer attached.Close()

	stderr := &cappedBuffer{limit: execStderrCap}
	if _, err := stdcopy.StdCopy(pw, stderr, attached.Reader); err != nil {
		_ = pw.CloseWithError(fmt.Errorf("docker: exec %v in %s: read output: %w", cmd, containerID, err))
		return
	}

	inspect, err := c.cli.ContainerExecInspect(ctx, execID)
	if err != nil {
		_ = pw.CloseWithError(fmt.Errorf("docker: exec %v in %s: inspect after completion: %w", cmd, containerID, err))
		return
	}
	if inspect.ExitCode != 0 {
		_ = pw.CloseWithError(&ExecExitError{
			Cmd:       cmd,
			Container: containerID,
			ExitCode:  inspect.ExitCode,
			Stderr:    strings.TrimSpace(stderr.buf.String()),
		})
		return
	}
	_ = pw.Close()
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
