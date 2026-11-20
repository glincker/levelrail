package build

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	bkclient "github.com/moby/buildkit/client"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/build/buildkitd"
)

const (
	// EnvBuildkitProbeTimeout bounds the check that the engine serves /grpc.
	EnvBuildkitProbeTimeout = "APP_BUILDKIT_PROBE_TIMEOUT"
	// EnvBuildkitReadyTimeout bounds the wait for the managed builder to answer.
	EnvBuildkitReadyTimeout = "APP_BUILDKIT_READY_TIMEOUT"
	// EnvBuildkitInfoTimeout bounds one readiness call to the managed builder.
	EnvBuildkitInfoTimeout = "APP_BUILDKIT_INFO_TIMEOUT"
	// EnvBuildkitRetryInterval spaces readiness calls while the builder starts.
	EnvBuildkitRetryInterval = "APP_BUILDKIT_RETRY_INTERVAL"

	defaultBuildkitProbeTimeout  = 10 * time.Second
	defaultBuildkitReadyTimeout  = 90 * time.Second
	defaultBuildkitInfoTimeout   = 5 * time.Second
	defaultBuildkitRetryInterval = time.Second

	execStderrCap = 1024
)

// ErrManagedBuilderUnavailable means the engine has no embedded BuildKit and
// the managed builder container could not be started either.
var ErrManagedBuilderUnavailable = errors.New("build: this engine has no embedded BuildKit and the managed builder container could not be started. " +
	"On Ubuntu 24.04, kernel.apparmor_restrict_unprivileged_userns=1 can block rootless BuildKit")

// noGRPCUpgrade matches the dial failure of an engine that answers the /grpc
// upgrade with a plain HTTP error instead of switching protocols.
var noGRPCUpgrade = regexp.MustCompile(`unable to upgrade to h2c, received (404|400)\b`)

func envDuration(name string, def time.Duration) time.Duration {
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv(name))); err == nil && d > 0 {
		return d
	}
	return def
}

// FallbackTimeouts are the timings of the managed builder fallback.
type FallbackTimeouts struct{ Probe, Ready, Info, Retry time.Duration }

// WithFallbackTimeouts reads the timings live from the runtime settings;
// a zero field keeps the environment seeded default.
func WithFallbackTimeouts(get func(context.Context) FallbackTimeouts) Option {
	return func(c *Client) { c.fallbackTimeouts = get }
}

func resolveTimeouts(ctx context.Context, get func(context.Context) FallbackTimeouts) FallbackTimeouts {
	t := FallbackTimeouts{
		Probe: envDuration(EnvBuildkitProbeTimeout, defaultBuildkitProbeTimeout),
		Ready: envDuration(EnvBuildkitReadyTimeout, defaultBuildkitReadyTimeout),
		Info:  envDuration(EnvBuildkitInfoTimeout, defaultBuildkitInfoTimeout),
		Retry: envDuration(EnvBuildkitRetryInterval, defaultBuildkitRetryInterval),
	}
	if get == nil {
		return t
	}
	live := get(ctx)
	for _, f := range []struct {
		dst *time.Duration
		v   time.Duration
	}{{&t.Probe, live.Probe}, {&t.Ready, live.Ready}, {&t.Info, live.Info}, {&t.Retry, live.Retry}} {
		if f.v > 0 {
			*f.dst = f.v
		}
	}
	return t
}

// probeEmbeddedBuildkit reports whether bk answers a control call in time.
func probeEmbeddedBuildkit(ctx context.Context, bk *bkclient.Client, timeout time.Duration) error {
	pctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	_, err := bk.Info(pctx)
	return err
}

// definitiveNoGRPC is true only when the engine itself said it has no BuildKit
// endpoint. Timeouts and other transient errors must not create anything.
func definitiveNoGRPC(err error) bool {
	if err == nil {
		return false
	}
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case grpccodes.Unimplemented:
			return true
		case grpccodes.Unavailable:
			return noGRPCUpgrade.MatchString(st.Message())
		}
	}
	return false
}

// managedStarter starts the managed builder and returns a connected client.
type managedStarter func(ctx context.Context, cfg buildkitd.Config) (*bkclient.Client, error)

// maybeFallback decides what a failed embedded probe means. It creates
// nothing unless the answer was definitive and a pinned image is configured.
func maybeFallback(ctx context.Context, probeErr error, get func(string) string, podman bool, start managedStarter) (*bkclient.Client, error) {
	if !definitiveNoGRPC(probeErr) {
		return nil, fmt.Errorf("build: probe buildkit via docker daemon: %w", probeErr)
	}
	if strings.TrimSpace(get("APP_BUILDKIT_IMAGE")) == "" {
		return nil, ErrBuildEngineUnsupported
	}
	cfg, err := buildkitd.SettingsFromEnv(get)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrManagedBuilderUnavailable, err)
	}
	if err := buildkitd.CheckPinned(cfg.Image); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrBuildEngineUnsupported, err)
	}
	if podman && !buildkitd.IsRootlessImage(cfg.Image) {
		return nil, ErrBuildEngineUnsupported
	}
	// Detached from the caller's short connect budget: the pull and first
	// start can take minutes. The ready timeout below still bounds it.
	return start(context.WithoutCancel(ctx), cfg)
}

// newManagedBuilderClient ensures the managed builder through the Engine API
// only and returns a client that reaches it by running "buildctl dial-stdio"
// inside it, so no docker CLI, host socket or published port is needed.
func newManagedBuilderClient(ctx context.Context, docker *dockerclient.Client, cfg buildkitd.Config, t FallbackTimeouts) (*bkclient.Client, error) {
	mgr, err := buildkitd.NewAPIManager(cfg, buildkitd.NewDockerEngine(docker))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrManagedBuilderUnavailable, err)
	}
	ctx, cancel := context.WithTimeout(ctx, t.Ready)
	defer cancel()
	if err := mgr.Ensure(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrManagedBuilderUnavailable, err)
	}
	var dialMu sync.Mutex
	dial := func(dctx context.Context, _ string) (net.Conn, error) {
		// Serialized so a removed container is recreated once, not per stream.
		dialMu.Lock()
		defer dialMu.Unlock()
		if err := mgr.Ensure(dctx); err != nil {
			return nil, err
		}
		return dialBuildctlStdio(dctx, docker, buildkitd.ContainerName)
	}
	bk, err := bkclient.New(ctx, "", bkclient.WithContextDialer(dial))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrManagedBuilderUnavailable, err)
	}
	infoTO, retry := t.Info, t.Retry
	var lastErr error
	for {
		ictx, icancel := context.WithTimeout(ctx, infoTO)
		_, lastErr = bk.Info(ictx)
		icancel()
		if lastErr == nil {
			return bk, nil
		}
		select {
		case <-ctx.Done():
			_ = bk.Close()
			return nil, fmt.Errorf("%w: not ready: %w", ErrManagedBuilderUnavailable, errors.Join(lastErr, ctx.Err()))
		case <-time.After(retry):
		}
	}
}

// dialBuildctlStdio opens one exec session whose stdin and stdout carry the
// BuildKit gRPC stream.
func dialBuildctlStdio(ctx context.Context, docker *dockerclient.Client, name string) (net.Conn, error) {
	ex, err := docker.ContainerExecCreate(ctx, name, container.ExecOptions{
		Cmd: []string{"buildctl", "dial-stdio"}, AttachStdin: true, AttachStdout: true, AttachStderr: true,
	})
	if err != nil {
		return nil, fmt.Errorf("build: create buildctl exec: %w", err)
	}
	hj, err := docker.ContainerExecAttach(ctx, ex.ID, container.ExecAttachOptions{})
	if err != nil {
		detail := ""
		if ins, ierr := docker.ContainerExecInspect(ctx, ex.ID); ierr == nil {
			detail = fmt.Sprintf(" (exec running=%t exit=%d)", ins.Running, ins.ExitCode)
		}
		return nil, fmt.Errorf("build: attach buildctl exec%s: %w", detail, err)
	}
	return newStdioConn(hj.Conn, hj.Reader), nil
}

// stdioConn adapts a multiplexed exec stream to a net.Conn. Engine streams
// without a TTY interleave stdout and stderr, so reads go through stdcopy.
type stdioConn struct {
	w    io.WriteCloser
	pr   *io.PipeReader
	once sync.Once
}

type closeWriter interface{ CloseWrite() error }

// capBuffer keeps only the first n bytes written.
type capBuffer struct {
	buf bytes.Buffer
	n   int
}

func (c *capBuffer) Write(p []byte) (int, error) {
	if room := c.n - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	c.n += int64(len(p))
	return c.w.Write(p)
}

func newStdioConn(raw io.WriteCloser, src io.Reader) *stdioConn {
	pr, pw := io.Pipe()
	go func() {
		errBuf := &capBuffer{n: execStderrCap}
		out := &countWriter{w: pw}
		_, err := stdcopy.StdCopy(out, errBuf, src)
		// A stream that ends before any payload means buildctl failed to start.
		if err == nil && out.n == 0 && errBuf.buf.Len() > 0 {
			err = fmt.Errorf("build: buildctl dial-stdio exited: %s", strings.TrimSpace(errBuf.buf.String()))
		}
		_ = pw.CloseWithError(err)
	}()
	return &stdioConn{w: raw, pr: pr}
}

func (c *stdioConn) Read(p []byte) (int, error)  { return c.pr.Read(p) }
func (c *stdioConn) Write(p []byte) (int, error) { return c.w.Write(p) }

func (c *stdioConn) Close() error {
	var err error
	c.once.Do(func() {
		if cw, ok := c.w.(closeWriter); ok {
			_ = cw.CloseWrite()
		}
		_ = c.pr.Close()
		err = c.w.Close()
	})
	return err
}

type stdioAddr struct{}

func (stdioAddr) Network() string { return "exec" }
func (stdioAddr) String() string  { return "buildctl-dial-stdio" }

func (c *stdioConn) LocalAddr() net.Addr              { return stdioAddr{} }
func (c *stdioConn) RemoteAddr() net.Addr             { return stdioAddr{} }
func (c *stdioConn) SetDeadline(time.Time) error      { return nil }
func (c *stdioConn) SetReadDeadline(time.Time) error  { return nil }
func (c *stdioConn) SetWriteDeadline(time.Time) error { return nil }

// RemoveManagedBuilder deletes the managed builder container and its cache
// volume, for tests and operator cleanup.
func RemoveManagedBuilder(ctx context.Context, docker *dockerclient.Client) error {
	err := docker.ContainerRemove(ctx, buildkitd.ContainerName, container.RemoveOptions{Force: true})
	if err != nil && !isNotFound(err) {
		return err
	}
	if err := docker.VolumeRemove(ctx, buildkitd.VolumeName, true); err != nil && !isNotFound(err) {
		return err
	}
	return nil
}

func isNotFound(err error) bool { return cerrdefs.IsNotFound(err) }
