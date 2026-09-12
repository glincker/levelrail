package agent

// This file: the agent-side half of the remote exec protocol, the one
// operation that spans many frames in both directions rather than
// fitting Execute's single request/response dispatch (execute.go).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// errExecCancelled ends the agent's side of an exec the control plane
// gave up on, so a blocked local read or stdin write unblocks promptly
// instead of outliving the caller that asked for the command.
var errExecCancelled = errors.New("agent: exec cancelled by the control plane")

// ErrTTYUnsupported is what an ExecRequest asking for a PTY gets when
// this agent's runtime has no interactive exec to offer.
var ErrTTYUnsupported = errors.New("agent: this node's container runtime does not support interactive exec")

// ExecRelay owns the agent side of every in-flight remote exec on one
// Session: it runs the command against the local docker.Runtime, streams
// its output back frame by frame under the control plane's flow-control
// window, feeds arriving stdin frames into the command as they land, and
// stops the local exec when the control plane cancels or the session
// ends. Unlike every other operation, an exec spans many frames in both
// directions, so it needs per-session state Execute's stateless dispatch
// cannot hold.
type ExecRelay struct {
	rt   docker.Runtime
	send func(*agentpb.AgentMessage)

	mu   sync.Mutex
	live map[string]*agentExec
}

// NewExecRelay builds a relay running commands against rt and writing
// its frames through send, which must serialize writes to the Session
// stream.
func NewExecRelay(rt docker.Runtime, send func(*agentpb.AgentMessage)) *ExecRelay {
	return &ExecRelay{rt: rt, send: send, live: make(map[string]*agentExec)}
}

// agentExec is one in-flight exec's agent-side state.
type agentExec struct {
	cancel context.CancelFunc
	in     chan *agentpb.ExecInput
	credit chan uint32
	stdin  *io.PipeWriter // nil unless the request attached stdin
	// resizes carries the newest requested PTY size to this exec's own
	// resize goroutine. Capacity one, and a full channel has its stale
	// entry dropped rather than blocking: only the latest size matters,
	// and Resize runs on the session's receive loop.
	resizes chan docker.TTYSize

	mu        sync.Mutex
	out       io.ReadCloser
	tty       docker.ExecSession // nil unless the request asked for a PTY
	cancelled bool
}

// attach records the local exec's output stream so a later cancel can
// close it. Reports false if the exec was already cancelled while the
// attach was still in flight, leaving the caller to close rc itself.
func (e *agentExec) attach(rc io.ReadCloser) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancelled {
		return false
	}
	e.out = rc
	return true
}

// attachTTY is attach for a PTY session, additionally keeping the
// session itself so resize frames have somewhere to land.
func (e *agentExec) attachTTY(sess docker.ExecSession) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancelled {
		return false
	}
	e.out = sess
	e.tty = sess
	return true
}

func (e *agentExec) isCancelled() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.cancelled
}

// stop ends the local exec. Closing the output stream is what actually
// unblocks a read still waiting on a command producing nothing, since
// docker.Client's own exec does not wire ctx into that read.
func (e *agentExec) stop() {
	e.mu.Lock()
	e.cancelled = true
	rc := e.out
	e.mu.Unlock()

	e.cancel()
	if rc != nil {
		_ = rc.Close()
	}
	if e.stdin != nil {
		_ = e.stdin.CloseWithError(errExecCancelled)
	}
}

// Start begins the exec identified by execID (the request's own
// RequestId, which also tags every frame of this exec). It returns
// immediately: attaching to the container is a Docker round trip, and
// doing it inline would stall the session's whole receive loop. The
// acknowledgment, or the error that stopped the exec from attaching, is
// sent once that round trip finishes.
func (r *ExecRelay) Start(ctx context.Context, execID string, req *agentpb.ExecRequest) {
	execCtx, cancel := context.WithCancel(ctx)
	// One frame past the window: a control plane honoring its credit can
	// have execWindowFrames stdin frames outstanding plus the single
	// terminal one, and this must accept all of them without ever
	// blocking the session's receive loop.
	ex := &agentExec{
		cancel:  cancel,
		in:      make(chan *agentpb.ExecInput, execWindowFrames+1),
		credit:  make(chan uint32, execWindowFrames+1),
		resizes: make(chan docker.TTYSize, 1),
	}

	var stdinR *io.PipeReader
	// A PTY is inherently interactive, so it always has a stdin side
	// whether or not the request bothered to say so.
	if req.GetAttachStdin() || req.GetTty() {
		stdinR, ex.stdin = io.Pipe()
	}

	r.mu.Lock()
	r.live[execID] = ex
	r.mu.Unlock()

	go r.run(execCtx, execID, ex, req, stdinR)
	if ex.stdin != nil {
		go r.pumpStdin(execCtx, execID, ex)
	}
}

func (r *ExecRelay) run(ctx context.Context, execID string, ex *agentExec, req *agentpb.ExecRequest, stdinR *io.PipeReader) {
	rc, err := r.attachLocal(ctx, ex, req, stdinR)
	if err != nil {
		if discarded := r.discard(execID); discarded != nil {
			discarded.stop()
		}
		r.send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{
			Response: &agentpb.AgentResponse{RequestId: execID, Error: err.Error()},
		}})
		return
	}
	if rc == nil {
		return
	}

	r.send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{
		Response: &agentpb.AgentResponse{RequestId: execID, Result: emptyResult()},
	}})

	r.relayOutput(ctx, execID, ex, rc)
	r.discard(execID)
	// stop, not just Close: an exec whose command exited while its stdin
	// was still arriving leaves pumpStdin blocked on a pipe write nobody
	// will ever read.
	ex.stop()
}

// attachLocal starts the local exec and records it on ex. A nil stream
// with a nil error means the control plane cancelled while the attach
// was still in flight, which attachLocal has already cleaned up after.
func (r *ExecRelay) attachLocal(ctx context.Context, ex *agentExec, req *agentpb.ExecRequest, stdinR *io.PipeReader) (io.ReadCloser, error) {
	if req.GetTty() {
		return r.attachTTY(ctx, ex, req, stdinR)
	}

	var (
		rc  io.ReadCloser
		err error
	)
	if stdinR != nil {
		rc, err = r.rt.ExecWithInput(ctx, req.GetContainerId(), req.GetCmd(), stdinR)
	} else {
		rc, err = r.rt.Exec(ctx, req.GetContainerId(), req.GetCmd())
	}
	if err != nil {
		return nil, err
	}
	if !ex.attach(rc) {
		_ = rc.Close()
		return nil, nil
	}
	return rc, nil
}

func (r *ExecRelay) attachTTY(ctx context.Context, ex *agentExec, req *agentpb.ExecRequest, stdinR *io.PipeReader) (io.ReadCloser, error) {
	tty, ok := r.rt.(docker.TTYRuntime)
	if !ok {
		return nil, ErrTTYUnsupported
	}
	sess, err := tty.ExecTTY(ctx, req.GetContainerId(), docker.ExecTTYOptions{
		Cmd:  req.GetCmd(),
		Env:  req.GetEnv(),
		Size: ttySizeFromPB(req.GetTtySize()),
	})
	if err != nil {
		return nil, err
	}
	if !ex.attachTTY(sess) {
		_ = sess.Close()
		return nil, nil
	}

	// pumpStdin already owns the credit accounting on the stdin side, so
	// the PTY gets fed from the same pipe every other exec uses rather
	// than a second, parallel flow-control path.
	if stdinR != nil {
		go func() {
			_, _ = io.Copy(sess, stdinR)
		}()
	}
	go r.applyResizes(ctx, ex, sess)
	return sess, nil
}

// applyResizes serializes one exec's terminal resizes onto its own
// goroutine, since each one is a round trip to the daemon that must not
// happen on the session's receive loop.
func (r *ExecRelay) applyResizes(ctx context.Context, ex *agentExec, sess docker.ExecSession) {
	for {
		select {
		case <-ctx.Done():
			return
		case size := <-ex.resizes:
			// A failed resize costs a wrongly sized terminal until the
			// next one, never the session.
			_ = sess.Resize(ctx, size)
		}
	}
}

// Resize records the newest terminal size for a PTY exec. A no-op for an
// exec that never asked for one, or that has already ended.
func (r *ExecRelay) Resize(rz *agentpb.ExecResize) {
	ex := r.lookup(rz.GetExecId())
	if ex == nil {
		return
	}
	size := docker.TTYSize{Rows: clampTTYDimension(rz.GetRows()), Cols: clampTTYDimension(rz.GetCols())}
	select {
	case ex.resizes <- size:
	default:
		select {
		case <-ex.resizes:
		default:
		}
		select {
		case ex.resizes <- size:
		default:
		}
	}
}

// relayOutput streams rc back as ExecOutput frames, sending at most
// execWindowFrames data frames before waiting for the control plane to
// credit consumed ones back. Pausing here is what pushes backpressure all
// the way down to the command: the local exec's own pipe fills once this
// stops reading, rather than this side buffering a dump the caller is not
// keeping up with.
func (r *ExecRelay) relayOutput(ctx context.Context, execID string, ex *agentExec, rc io.Reader) {
	buf := make([]byte, execChunkBytes)
	credit := uint32(execWindowFrames)

	for {
		if credit == 0 {
			select {
			case refund := <-ex.credit:
				credit += refund
			case <-ctx.Done():
				return
			}
			continue
		}

		n, err := rc.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			r.sendOutput(&agentpb.ExecOutput{ExecId: execID, Chunk: chunk})
			credit--
		}
		if err != nil {
			// A cancelled exec's read error describes the cancel, not the
			// command, and nobody is listening for it either way.
			if !ex.isCancelled() {
				r.sendOutput(terminalOutput(execID, err))
			}
			return
		}
	}
}

// pumpStdin feeds arriving stdin frames into the local exec's input pipe
// and credits them back as the command consumes them. It needs its own
// goroutine because that pipe write blocks until the command reads, which
// must never happen on the session's receive loop.
func (r *ExecRelay) pumpStdin(ctx context.Context, execID string, ex *agentExec) {
	consumed := uint32(0)

	for {
		select {
		case <-ctx.Done():
			_ = ex.stdin.CloseWithError(errExecCancelled)
			return
		case in := <-ex.in:
			switch {
			case in.GetError() != "":
				r.failExec(execID, ex, fmt.Errorf("agent: exec stdin source failed: %s", in.GetError()))
				return
			case in.GetEof():
				_ = ex.stdin.Close()
				return
			default:
				if _, err := ex.stdin.Write(in.GetChunk()); err != nil {
					_ = ex.stdin.Close()
					return
				}
				consumed++
				if consumed >= execCreditBatch {
					r.send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_ExecCredit{
						ExecCredit: &agentpb.ExecCredit{ExecId: execID, Frames: consumed},
					}})
					consumed = 0
				}
			}
		}
	}
}

// Input routes one stdin frame to its exec.
func (r *ExecRelay) Input(in *agentpb.ExecInput) {
	execID := in.GetExecId()
	ex := r.lookup(execID)
	if ex == nil {
		return
	}
	select {
	case ex.in <- in:
	default:
		r.failExec(execID, ex, ErrExecWindowViolated)
	}
}

// Credit records the control plane's refund of this exec's output window.
func (r *ExecRelay) Credit(c *agentpb.ExecCredit) {
	execID := c.GetExecId()
	ex := r.lookup(execID)
	if ex == nil {
		return
	}
	select {
	case ex.credit <- c.GetFrames():
	default:
		r.failExec(execID, ex, ErrExecWindowViolated)
	}
}

// Cancel stops an exec the control plane no longer wants. No terminal
// frame follows: the caller that cancelled is not reading anymore.
func (r *ExecRelay) Cancel(execID string) {
	if ex := r.discard(execID); ex != nil {
		ex.stop()
	}
}

// CloseAll stops every in-flight exec, for a session that is ending.
func (r *ExecRelay) CloseAll() {
	r.mu.Lock()
	live := r.live
	r.live = make(map[string]*agentExec)
	r.mu.Unlock()

	for _, ex := range live {
		ex.stop()
	}
}

// failExec ends an exec with err and reports it as that stream's one
// terminal frame, for a failure the output relay itself cannot observe
// (a stdin source that died upstream, a peer overrunning its window).
func (r *ExecRelay) failExec(execID string, ex *agentExec, err error) {
	if r.discard(execID) == nil {
		return
	}
	r.sendOutput(&agentpb.ExecOutput{ExecId: execID, Failure: &agentpb.ExecFailure{Message: err.Error()}})
	ex.stop()
}

func (r *ExecRelay) lookup(execID string) *agentExec {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.live[execID]
}

// discard removes execID from the live set, returning it only to the
// first caller, so exactly one of them sends its terminal frame.
func (r *ExecRelay) discard(execID string) *agentExec {
	r.mu.Lock()
	defer r.mu.Unlock()
	ex, ok := r.live[execID]
	if !ok {
		return nil
	}
	delete(r.live, execID)
	return ex
}

func (r *ExecRelay) sendOutput(out *agentpb.ExecOutput) {
	r.send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_ExecOutput{ExecOutput: out}})
}

// terminalOutput turns the error that ended a local exec stream into its
// one terminal frame. A non-zero exit is carried as structured fields,
// not just a message, so the control plane can rebuild the typed
// docker.ExecExitError its own callers branch on.
func terminalOutput(execID string, err error) *agentpb.ExecOutput {
	if errors.Is(err, io.EOF) {
		return &agentpb.ExecOutput{ExecId: execID, Eof: true}
	}
	failure := &agentpb.ExecFailure{Message: err.Error()}
	var exitErr *docker.ExecExitError
	if errors.As(err, &exitErr) {
		failure.Exit = &agentpb.ExecExit{
			Cmd:       exitErr.Cmd,
			Container: exitErr.Container,
			ExitCode:  int32(exitErr.ExitCode), //nolint:gosec // a process exit code always fits in an int32
			Stderr:    exitErr.Stderr,
		}
	}
	return &agentpb.ExecOutput{ExecId: execID, Failure: failure}
}
