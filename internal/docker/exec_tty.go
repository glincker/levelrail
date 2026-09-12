package docker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	dockerclient "github.com/docker/docker/client"
)

var _ TTYRuntime = (*Client)(nil)

// execInspectTimeout bounds the exit-code lookup ttySession.Read makes
// once its stream ends. Read has no context of its own, and a hung
// daemon must not wedge the last Read of a session forever.
const execInspectTimeout = 10 * time.Second

// ttyHangUp is what Close sends before tearing the connection down:
// ETX then EOT, the interrupt and end-of-input a terminal user types as
// Ctrl-C then Ctrl-D. The Engine API has no "kill this exec" call and
// dropping the connection alone leaves the process running
// (moby/moby#9098), so this is the only lever a client has. A shell
// exits on it; a program that ignores both outlives the session, the
// same residual gap `docker exec` itself has.
var ttyHangUp = []byte{0x03, 0x04}

// ExecTTY implements TTYRuntime. Unlike Exec, the attach stream is raw
// rather than stdcopy-multiplexed: Docker merges stdout and stderr onto
// the PTY exactly as a real terminal does, so there is nothing to
// demultiplex and the bytes pass straight through in both directions.
func (c *Client) ExecTTY(ctx context.Context, containerID string, opts ExecTTYOptions) (ExecSession, error) {
	if len(opts.Cmd) == 0 {
		return nil, fmt.Errorf("docker: exec tty in %s: no command given", containerID)
	}

	createOpts := container.ExecOptions{
		Cmd:          opts.Cmd,
		Env:          opts.Env,
		User:         opts.User,
		WorkingDir:   opts.WorkingDir,
		Tty:          true,
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}
	if opts.Size.Rows > 0 && opts.Size.Cols > 0 {
		createOpts.ConsoleSize = &[2]uint{uint(opts.Size.Rows), uint(opts.Size.Cols)}
	}

	created, err := c.cli.ContainerExecCreate(ctx, containerID, createOpts)
	if err != nil {
		return nil, fmt.Errorf("docker: exec tty create %v in %s: %w", opts.Cmd, containerID, err)
	}

	attached, err := c.cli.ContainerExecAttach(ctx, created.ID, container.ExecAttachOptions{
		Tty:         true,
		ConsoleSize: createOpts.ConsoleSize,
	})
	if err != nil {
		return nil, fmt.Errorf("docker: exec tty attach %v in %s: %w", opts.Cmd, containerID, err)
	}

	return &ttySession{
		cli:       c.cli,
		execID:    created.ID,
		container: containerID,
		cmd:       opts.Cmd,
		attached:  attached,
		closed:    make(chan struct{}),
	}, nil
}

// ttySession is ExecTTY's ExecSession: the hijacked PTY connection plus
// the exec ID needed to resize it and to recover its exit code.
type ttySession struct {
	cli       *dockerclient.Client
	execID    string
	container string
	cmd       []string
	attached  dockertypes.HijackedResponse

	closeOnce sync.Once
	closed    chan struct{}

	// pending holds a read error that arrived alongside final bytes, so
	// those bytes reach the caller before the error does. Read-only
	// outside Read, which io.Reader's contract already makes
	// single-goroutine.
	pending error
}

func (s *ttySession) Read(p []byte) (int, error) {
	if s.pending != nil {
		err := s.pending
		s.pending = nil
		return 0, s.resolve(err)
	}
	n, err := s.attached.Reader.Read(p)
	if err == nil {
		return n, nil
	}
	if n > 0 {
		s.pending = err
		return n, nil
	}
	return 0, s.resolve(err)
}

func (s *ttySession) Write(p []byte) (int, error) {
	n, err := s.attached.Conn.Write(p)
	if err != nil {
		return n, fmt.Errorf("docker: exec tty %v in %s: write input: %w", s.cmd, s.container, err)
	}
	return n, nil
}

// Resize applies size to the running exec's PTY, which is what makes the
// process inside see a SIGWINCH and redraw.
func (s *ttySession) Resize(ctx context.Context, size TTYSize) error {
	if size.Rows == 0 || size.Cols == 0 {
		return fmt.Errorf("docker: exec tty resize %s: rows and cols must both be positive", s.execID)
	}
	if err := s.cli.ContainerExecResize(ctx, s.execID, container.ResizeOptions{
		Height: uint(size.Rows),
		Width:  uint(size.Cols),
	}); err != nil {
		return fmt.Errorf("docker: exec tty resize %s: %w", s.execID, err)
	}
	return nil
}

func (s *ttySession) Close() error {
	s.closeOnce.Do(func() {
		close(s.closed)
		_, _ = s.attached.Conn.Write(ttyHangUp)
		_ = s.attached.CloseWrite()
		s.attached.Close()
	})
	return nil
}

// resolve turns the end of the PTY stream into this session's final
// error: the command's own non-zero exit where there is one, io.EOF for
// a clean end, and io.EOF as well for the torn-down connection a Close
// causes, since that end was the caller's own doing.
func (s *ttySession) resolve(err error) error {
	select {
	case <-s.closed:
		return io.EOF
	default:
	}
	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("docker: exec tty %v in %s: read output: %w", s.cmd, s.container, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), execInspectTimeout)
	defer cancel()
	inspect, inspectErr := s.cli.ContainerExecInspect(ctx, s.execID)
	if inspectErr != nil {
		return fmt.Errorf("docker: exec tty %v in %s: inspect after completion: %w", s.cmd, s.container, inspectErr)
	}
	if inspect.ExitCode != 0 {
		return &ExecExitError{Cmd: s.cmd, Container: s.container, ExitCode: inspect.ExitCode}
	}
	return io.EOF
}
