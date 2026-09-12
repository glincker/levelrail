package agent

// This file: GRPCTransport, the control-plane side of the
// reverse-dialed connection. An agent.Transport
// implemented by dispatching every docker.Runtime-shaped call over a
// real agent's Session stream via mux (mux.go), instead of Local's
// direct in-process call.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// GRPCTransport implements Transport by dispatching every call over an
// established Session stream to a real, remote agent. Built from a
// *mux, not a raw stream: the request/response multiplexing this type
// depends on is already handled underneath it. Unexported constructor
// (newGRPCTransport): callers get one of these from the control plane's
// Session handler once an agent's stream is accepted and authenticated
// (server.go's remaining wiring), never constructed
// directly.
type GRPCTransport struct {
	mux *mux
}

func newGRPCTransport(m *mux) *GRPCTransport {
	return &GRPCTransport{mux: m}
}

var (
	_ Transport          = (*GRPCTransport)(nil)
	_ docker.TTYRuntime  = (*GRPCTransport)(nil)
	_ docker.ExecSession = (*execTTYStream)(nil)
)

// InspectByName implements Transport (docker.Runtime).
func (t *GRPCTransport) InspectByName(ctx context.Context, name string) (*docker.ContainerState, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_InspectByName{InspectByName: &agentpb.InspectByNameRequest{Name: name}},
	})
	if err != nil {
		return nil, err
	}
	got := resp.GetInspectByName()
	if got == nil || !got.GetFound() {
		return nil, nil
	}
	return containerStateFromPB(got.GetState()), nil
}

// Create implements Transport (docker.Runtime).
func (t *GRPCTransport) Create(ctx context.Context, spec docker.ContainerSpec) (string, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Create{Create: &agentpb.CreateRequest{Spec: containerSpecToPB(spec)}},
	})
	if err != nil {
		return "", err
	}
	return resp.GetCreate().GetId(), nil
}

// Start implements Transport (docker.Runtime).
func (t *GRPCTransport) Start(ctx context.Context, id string) error {
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: id}},
	})
	return err
}

// Stop implements Transport (docker.Runtime). timeout < 0 (wait
// indefinitely, docker.Runtime.Stop's own convention) is carried across
// the wire as TimeoutMs: -1, not clamped to 0, so the agent applies the
// identical "wait indefinitely" semantics rather than a silently
// different one.
func (t *GRPCTransport) Stop(ctx context.Context, id string, timeout time.Duration) error {
	ms := timeout.Milliseconds()
	if timeout < 0 {
		ms = -1
	}
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Stop{Stop: &agentpb.StopRequest{Id: id, TimeoutMs: ms}},
	})
	return err
}

// Remove implements Transport (docker.Runtime).
func (t *GRPCTransport) Remove(ctx context.Context, id string, force bool) error {
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Remove{Remove: &agentpb.RemoveRequest{Id: id, Force: force}},
	})
	return err
}

// UpdateResources implements Transport (docker.Runtime).
func (t *GRPCTransport) UpdateResources(ctx context.Context, id string, resources docker.Resources) error {
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_UpdateResources{UpdateResources: &agentpb.UpdateResourcesRequest{
			Id:        id,
			Resources: resourcesToPB(&resources),
		}},
	})
	return err
}

// ListImages implements Transport (docker.Runtime).
func (t *GRPCTransport) ListImages(ctx context.Context, repo string) ([]docker.ImageInfo, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_ListImages{ListImages: &agentpb.ListImagesRequest{Repo: repo}},
	})
	if err != nil {
		return nil, err
	}
	return imageInfosFromPB(resp.GetListImages().GetImages()), nil
}

// ListByPrefix implements Transport (docker.Runtime).
func (t *GRPCTransport) ListByPrefix(ctx context.Context, prefix string) ([]docker.ContainerState, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_ListByPrefix{ListByPrefix: &agentpb.ListByPrefixRequest{Prefix: prefix}},
	})
	if err != nil {
		return nil, err
	}
	return containerStatesFromPB(resp.GetListByPrefix().GetContainers()), nil
}

// Exec implements Transport (docker.Runtime). The initial round trip is
// synchronous, matching docker.Client.Exec's own behavior: an exec that
// cannot attach (no such container, daemon refused) fails here rather
// than on a later Read. Output arrives afterwards as ExecOutput frames,
// reassembled by the returned stream.
func (t *GRPCTransport) Exec(ctx context.Context, containerID string, cmd []string) (io.ReadCloser, error) {
	return t.startExec(ctx, containerID, cmd, nil, execAttach{})
}

// ExecWithInput implements Transport (docker.Runtime). Exec plus a stdin
// direction: a goroutine drains stdin into ExecInput frames while output
// frames stream back, the same concurrency docker.Client.ExecWithInput
// needs locally to keep a command that writes while it reads from
// deadlocking.
func (t *GRPCTransport) ExecWithInput(ctx context.Context, containerID string, cmd []string, stdin io.Reader) (io.ReadCloser, error) {
	if stdin == nil {
		// The remote command gets a real stdin pipe either way, so it
		// needs an EOF from somewhere or it waits forever.
		stdin = bytes.NewReader(nil)
	}
	return t.startExec(ctx, containerID, cmd, stdin, execAttach{stdin: true})
}

// ExecTTY implements docker.TTYRuntime. The PTY lives on the agent's
// side; this end is the same windowed frame stream every other exec
// uses, plus a resize frame, so a remote terminal and a local one differ
// only in how far the bytes travel.
func (t *GRPCTransport) ExecTTY(ctx context.Context, containerID string, opts docker.ExecTTYOptions) (docker.ExecSession, error) {
	// The caller drives input by calling Write, so the pipe stands in for
	// the source reader an ordinary exec would have handed startExec, and
	// pumpStdin's existing credit accounting covers the terminal too.
	inR, inW := io.Pipe()

	rc, err := t.startExec(ctx, containerID, opts.Cmd, inR, execAttach{stdin: true, tty: true, opts: opts})
	if err != nil {
		_ = inW.Close()
		return nil, err
	}

	stream, ok := rc.(*execStream)
	if !ok {
		_ = inW.Close()
		_ = rc.Close()
		return nil, errors.New("agent: exec tty: unexpected stream type")
	}
	sess := &execTTYStream{execStream: stream, in: inW}
	go sess.closeInputOnEnd()
	return sess, nil
}

// execAttach is startExec's set of exec-shape options, grouped rather
// than passed as a run of bare booleans at the call site.
type execAttach struct {
	stdin bool
	tty   bool
	opts  docker.ExecTTYOptions
}

func (t *GRPCTransport) startExec(ctx context.Context, containerID string, cmd []string, stdin io.Reader, attach execAttach) (io.ReadCloser, error) {
	execID, err := randomRequestID()
	if err != nil {
		return nil, err
	}

	// Subscribe before issuing the call: the agent starts relaying output
	// the moment it has attached, which can be before this side has even
	// seen the acknowledgment.
	sub := t.mux.subscribeExec(execID)
	if sub == nil {
		return nil, ErrSessionClosed
	}

	req := &agentpb.ExecRequest{
		ContainerId: containerID,
		Cmd:         cmd,
		AttachStdin: attach.stdin,
		Tty:         attach.tty,
	}
	if attach.tty {
		req.Env = attach.opts.Env
		req.TtySize = ttySizeToPB(attach.opts.Size)
	}

	if _, callErr := t.mux.CallWithID(ctx, execID, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_Exec{Exec: req},
	}); callErr != nil {
		t.mux.unsubscribeExec(execID)
		// The acknowledgment may have failed only on this side (a caller
		// whose ctx expired mid-attach), so tell the agent to drop the
		// exec rather than leave it running for nobody.
		_ = t.mux.sendExecCancel(execID)
		return nil, callErr
	}

	s := &execStream{mux: t.mux, execID: execID, sub: sub, ctx: ctx, done: make(chan struct{})}
	go s.watchContext()
	if attach.stdin {
		go s.pumpStdin(stdin)
	}
	return s, nil
}

// errExecStreamClosed is what a Read blocked on a stream the caller
// closed returns, distinct from io.EOF (the command's own clean end).
var errExecStreamClosed = errors.New("agent: exec stream closed")

// execStream is the io.ReadCloser Exec and ExecWithInput return: it
// reassembles the agent's ExecOutput frames into a byte stream, refunds
// flow-control credit as it consumes them, and cancels the remote exec
// if the caller gives up before the command finishes.
type execStream struct {
	mux    *mux
	execID string
	sub    *execSub
	ctx    context.Context

	done      chan struct{}
	closeOnce sync.Once

	// Read's own state, single-goroutine by io.Reader's contract.
	rem      []byte
	finished error
	consumed uint32
}

func (s *execStream) Read(p []byte) (int, error) {
	for {
		if len(s.rem) > 0 {
			n := copy(p, s.rem)
			s.rem = s.rem[n:]
			return n, nil
		}
		if s.finished != nil {
			return 0, s.finished
		}

		select {
		case frame, ok := <-s.sub.out:
			if !ok {
				s.finished = s.sub.failure()
				if s.finished == nil {
					s.finished = ErrSessionClosed
				}
				s.finish()
				return 0, s.finished
			}
			if chunk := frame.GetChunk(); len(chunk) > 0 {
				s.rem = chunk
				s.creditOne()
				continue
			}
			if failure := frame.GetFailure(); failure != nil {
				s.finished = execFailureToError(failure)
				s.finish()
				return 0, s.finished
			}
			if frame.GetEof() {
				s.finished = io.EOF
				s.finish()
				return 0, io.EOF
			}
		case <-s.done:
			if err := s.ctx.Err(); err != nil {
				return 0, err
			}
			return 0, errExecStreamClosed
		case <-s.ctx.Done():
			return 0, s.ctx.Err()
		}
	}
}

// Close releases the subscription and, if the command is still running,
// tells the agent to stop it. Idempotent, and a no-op cancel once the
// stream already ended on its own.
func (s *execStream) Close() error {
	first := false
	s.closeOnce.Do(func() {
		first = true
		close(s.done)
	})
	if !first {
		return nil
	}
	s.mux.unsubscribeExec(s.execID)
	return s.mux.sendExecCancel(s.execID)
}

// finish marks the stream ended by the agent rather than by the caller,
// so a later Close neither cancels an already-finished exec nor reports
// an error for failing to.
func (s *execStream) finish() {
	s.mux.unsubscribeExec(s.execID)
	s.closeOnce.Do(func() { close(s.done) })
}

func (s *execStream) watchContext() {
	select {
	case <-s.ctx.Done():
		_ = s.Close()
	case <-s.done:
	}
}

// creditOne refunds the agent's output window in batches: without it the
// agent stops sending once execWindowFrames frames are unacknowledged.
func (s *execStream) creditOne() {
	s.consumed++
	if s.consumed < execCreditBatch {
		return
	}
	frames := s.consumed
	s.consumed = 0
	// A failed credit means the session is gone, which the next Read
	// discovers on its own; there is nothing to recover here.
	_ = s.mux.sendExecCredit(s.execID, frames)
}

// pumpStdin drains r into ExecInput frames for as long as the agent's
// credit allows, then signals end of input. A read failure on r is sent
// as an explicit error rather than an EOF: a truncated dump must fail
// the restore, not look like a complete one.
func (s *execStream) pumpStdin(r io.Reader) {
	buf := make([]byte, execChunkBytes)
	credit := uint32(execWindowFrames)

	for {
		if credit == 0 {
			select {
			case refund, ok := <-s.sub.credit:
				if !ok {
					return
				}
				credit += refund
			case <-s.done:
				return
			case <-s.ctx.Done():
				return
			}
			continue
		}

		n, readErr := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if err := s.mux.sendExecInput(&agentpb.ExecInput{ExecId: s.execID, Chunk: chunk}); err != nil {
				return
			}
			credit--
		}
		if readErr != nil {
			final := &agentpb.ExecInput{ExecId: s.execID, Eof: true}
			if !errors.Is(readErr, io.EOF) {
				final = &agentpb.ExecInput{ExecId: s.execID, Error: readErr.Error()}
			}
			_ = s.mux.sendExecInput(final)
			return
		}
	}
}

// execTTYStream is ExecTTY's docker.ExecSession: an ordinary exec stream
// for the output direction, a pipe into pumpStdin for the input one, and
// a resize frame on top.
type execTTYStream struct {
	*execStream
	in *io.PipeWriter
}

func (s *execTTYStream) Write(p []byte) (int, error) {
	return s.in.Write(p)
}

func (s *execTTYStream) Resize(_ context.Context, size docker.TTYSize) error {
	return s.mux.sendExecResize(s.execID, size)
}

func (s *execTTYStream) Close() error {
	_ = s.in.Close()
	return s.execStream.Close()
}

// closeInputOnEnd unblocks a Write waiting on a session that already
// ended (the remote command exited, the caller closed), which would
// otherwise wait forever on a pipe pumpStdin has stopped draining.
func (s *execTTYStream) closeInputOnEnd() {
	<-s.done
	_ = s.in.CloseWithError(errExecStreamClosed)
}

func execFailureToError(f *agentpb.ExecFailure) error {
	if exit := f.GetExit(); exit != nil {
		return &docker.ExecExitError{
			Cmd:       exit.GetCmd(),
			Container: exit.GetContainer(),
			ExitCode:  int(exit.GetExitCode()),
			Stderr:    exit.GetStderr(),
		}
	}
	return errors.New(f.GetMessage())
}

// EnsureNetwork implements Transport (docker.Runtime).
func (t *GRPCTransport) EnsureNetwork(ctx context.Context, name string) (string, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_EnsureNetwork{EnsureNetwork: &agentpb.EnsureNetworkRequest{Name: name}},
	})
	if err != nil {
		return "", err
	}
	return resp.GetEnsureNetwork().GetId(), nil
}

// RemoveNetwork implements Transport (docker.Runtime).
func (t *GRPCTransport) RemoveNetwork(ctx context.Context, name string) error {
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_RemoveNetwork{RemoveNetwork: &agentpb.RemoveNetworkRequest{Name: name}},
	})
	return err
}

// ListNetworksByPrefix implements Transport (docker.Runtime).
func (t *GRPCTransport) ListNetworksByPrefix(ctx context.Context, prefix string) ([]docker.NetworkInfo, error) {
	resp, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_ListNetworksByPrefix{ListNetworksByPrefix: &agentpb.ListNetworksByPrefixRequest{Prefix: prefix}},
	})
	if err != nil {
		return nil, err
	}
	return networkInfosFromPB(resp.GetListNetworksByPrefix().GetNetworks()), nil
}

// EnsureVolume implements Transport (docker.Runtime).
func (t *GRPCTransport) EnsureVolume(ctx context.Context, name string) error {
	_, err := t.mux.Call(ctx, &agentpb.AgentRequest{
		Op: &agentpb.AgentRequest_EnsureVolume{EnsureVolume: &agentpb.EnsureVolumeRequest{Name: name}},
	})
	return err
}

// Events implements Transport (docker.Runtime). The one method that
// doesn't fit mux.Call's request/response shape: docker.Runtime.Events
// returns two channels streaming until ctx is cancelled, not one
// response. This issues a single WatchEvents request (acked immediately
// like any other Empty-result call, then relays every ProxiedEvent
// mux.subscribe's channel receives onto a fresh pair of channels this
// method returns, translating docker.Event's wire shape back and
// closing both channels once ctx is done or the subscription itself
// ends (session closed, agent-side stream error). ADR 003's push-not-
// poll event design, real end to end.
//
// Backpressure: mux's eventChanBuffer (64) bounds how far this can fall
// behind the agent's own event rate before frames start being dropped
// rather than blocking the shared recvLoop; a caller needing a
// guarantee stronger than "recent events, best effort" would need a
// different mechanism, not built here.
func (t *GRPCTransport) Events(ctx context.Context) (<-chan docker.Event, <-chan error) {
	events := make(chan docker.Event)
	errs := make(chan error, 1)

	watchID, err := randomRequestID() // same shape, a different logical ID space
	if err != nil {
		errs <- err
		close(events)
		close(errs)
		return events, errs
	}

	sub := t.mux.subscribe(watchID)
	if sub == nil {
		errs <- ErrSessionClosed
		close(events)
		close(errs)
		return events, errs
	}

	// The WatchEvents round trip itself happens inside the goroutine,
	// not before returning: docker.Client.Events (internal/docker)
	// returns its channels immediately and subscribes asynchronously,
	// and callers (internal/telemetry.LogCollector,
	// internal/reconcile.Engine.Run) rely on that non-blocking contract.
	// Blocking this call on a network round trip would be a real,
	// silent behavior change for exactly the same interface.
	go func() {
		defer close(events)
		defer close(errs)
		defer t.mux.unsubscribe(watchID)

		if _, callErr := t.mux.Call(ctx, &agentpb.AgentRequest{
			Op: &agentpb.AgentRequest_WatchEvents{WatchEvents: &agentpb.WatchEventsRequest{WatchId: watchID}},
		}); callErr != nil {
			errs <- callErr
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-sub:
				if !ok {
					return
				}
				events <- docker.Event{
					Action:        docker.EventAction(ev.GetAction()),
					ContainerName: ev.GetContainerName(),
					Time:          timestampFromPB(ev.GetTime()),
				}
			}
		}
	}()

	return events, errs
}
