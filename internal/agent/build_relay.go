package agent

// Agent-side half of the dispatched-build protocol: unpacks the streamed context and runs it on this node's BuildKit.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/build"
)

// errBuildCancelled ends a build the control plane gave up on, so the
// solve and the context unpack both stop instead of burning this node's
// CPU for a caller that is no longer listening.
var errBuildCancelled = errors.New("agent: build cancelled by the control plane")

// BuildRunner runs dispatched builds; *build.Client satisfies it.
type BuildRunner interface {
	SolveRemote(ctx context.Context, req build.RemoteRequest, out io.Writer, progress func(build.ProgressEvent)) (*build.Result, error)
}

// BuildRelay owns the agent side of every in-flight dispatched build on
// one Session, the same per-session state ExecRelay holds for exec.
type BuildRelay struct {
	runner BuildRunner
	send   func(*agentpb.AgentMessage)

	mu   sync.Mutex
	live map[string]*agentBuild
}

// NewBuildRelay returns a relay that runs builds through runner and writes frames through send; a nil runner rejects every build.
func NewBuildRelay(runner BuildRunner, send func(*agentpb.AgentMessage)) *BuildRelay {
	return &BuildRelay{runner: runner, send: send, live: make(map[string]*agentBuild)}
}

// agentBuild is one in-flight build's agent-side state.
type agentBuild struct {
	cancel  context.CancelFunc
	dir     string
	context *io.PipeWriter
	in      chan *agentpb.BuildInput
	credit  chan uint32

	// sendMu serializes this build's data frames; the progress relay and image exporter share one flow-control window.
	sendMu       sync.Mutex
	window       uint32
	terminalOnce sync.Once

	mu        sync.Mutex
	cancelled bool
}

func (b *agentBuild) isCancelled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cancelled
}

// stop ends the build and releases its unpacked context. Closing the
// context pipe is what unblocks an unpack still waiting on frames that
// will never arrive.
func (b *agentBuild) stop() {
	b.mu.Lock()
	b.cancelled = true
	b.mu.Unlock()

	b.cancel()
	_ = b.context.CloseWithError(errBuildCancelled)
	if b.dir != "" {
		_ = os.RemoveAll(b.dir)
	}
}

// sendData waits for credit when the window is exhausted, pushing backpressure into BuildKit's exporter.
func (b *agentBuild) sendData(ctx context.Context, send func(*agentpb.AgentMessage), frame *agentpb.BuildOutput) error {
	b.sendMu.Lock()
	defer b.sendMu.Unlock()

	for b.window == 0 {
		select {
		case refund := <-b.credit:
			b.window += refund
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_BuildOutput{BuildOutput: frame}})
	b.window--
	return nil
}

// sendTerminal sends the one terminal frame, outside the window; exactly one goes out per build.
func (b *agentBuild) sendTerminal(send func(*agentpb.AgentMessage), frame *agentpb.BuildOutput) {
	b.terminalOnce.Do(func() {
		b.sendMu.Lock()
		defer b.sendMu.Unlock()
		send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_BuildOutput{BuildOutput: frame}})
	})
}

// Start begins the build identified by buildID without blocking the session's receive loop.
func (r *BuildRelay) Start(ctx context.Context, buildID string, req *agentpb.BuildRequest) {
	if r.runner == nil {
		r.sendResponse(buildID, "agent: this node has no BuildKit connection, so it cannot run a dispatched build")
		return
	}

	remoteReq, err := buildRequestFromPB(req)
	if err != nil {
		r.sendResponse(buildID, err.Error())
		return
	}

	dir, err := os.MkdirTemp("", "levelrail-remote-build-*")
	if err != nil {
		r.sendResponse(buildID, fmt.Sprintf("agent: create build context dir: %v", err))
		return
	}
	remoteReq.ContextDir = dir

	buildCtx, cancel := context.WithCancel(ctx)
	contextR, contextW := io.Pipe()
	b := &agentBuild{
		cancel:  cancel,
		dir:     dir,
		context: contextW,
		// One frame past the window, the same slack ExecRelay reserves so
		// a control plane honoring its credit never blocks the session's
		// receive loop.
		in:     make(chan *agentpb.BuildInput, buildWindowFrames+1),
		credit: make(chan uint32, buildWindowFrames+1),
		window: buildWindowFrames,
	}

	r.mu.Lock()
	r.live[buildID] = b
	r.mu.Unlock()

	r.sendResponse(buildID, "")

	go r.pumpContext(buildCtx, buildID, b)
	go r.run(buildCtx, buildID, b, remoteReq, contextR)
}

// run unpacks the context the control plane is streaming up, then builds
// it, then reports exactly one terminal frame either way.
func (r *BuildRelay) run(ctx context.Context, buildID string, b *agentBuild, req build.RemoteRequest, contextR *io.PipeReader) {
	defer func() {
		r.discard(buildID)
		b.stop()
	}()

	if err := build.UntarContext(ctx, contextR, req.ContextDir); err != nil {
		_ = contextR.CloseWithError(err)
		r.fail(buildID, b, fmt.Errorf("agent: unpack build context: %w", err))
		return
	}
	_ = contextR.Close()

	res, err := r.runner.SolveRemote(ctx, req, &buildImageWriter{ctx: ctx, relay: r, build: b, buildID: buildID}, func(ev build.ProgressEvent) {
		// A progress frame nobody is waiting for is worth dropping, not
		// worth failing the build over.
		_ = b.sendData(ctx, r.send, &agentpb.BuildOutput{
			BuildId: buildID,
			Frame:   &agentpb.BuildOutput_Progress{Progress: buildProgressToPB(ev)},
		})
	})
	if err != nil {
		r.fail(buildID, b, err)
		return
	}

	b.sendTerminal(r.send, &agentpb.BuildOutput{
		BuildId: buildID,
		Frame: &agentpb.BuildOutput_Done{Done: &agentpb.BuildDone{
			DurationMs:       res.Duration.Milliseconds(),
			ExporterResponse: res.ExporterResponse,
		}},
	})
}

// pumpContext runs on its own goroutine because the unpack pipe write blocks until the unpack reads.
func (r *BuildRelay) pumpContext(ctx context.Context, buildID string, b *agentBuild) {
	consumed := uint32(0)

	for {
		select {
		case <-ctx.Done():
			_ = b.context.CloseWithError(errBuildCancelled)
			return
		case in := <-b.in:
			switch {
			case in.GetError() != "":
				_ = b.context.CloseWithError(fmt.Errorf("agent: build context source failed: %s", in.GetError()))
				return
			case in.GetEof():
				_ = b.context.Close()
				return
			default:
				if _, err := b.context.Write(in.GetChunk()); err != nil {
					return
				}
				consumed++
				if consumed >= buildCreditBatch {
					r.send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_BuildCredit{
						BuildCredit: &agentpb.BuildCredit{BuildId: buildID, Frames: consumed},
					}})
					consumed = 0
				}
			}
		}
	}
}

// Input routes one build-context frame to its build.
func (r *BuildRelay) Input(in *agentpb.BuildInput) {
	buildID := in.GetBuildId()
	b := r.lookup(buildID)
	if b == nil {
		return
	}
	select {
	case b.in <- in:
	default:
		r.failAndStop(buildID, b, ErrBuildWindowViolated)
	}
}

// Credit records the control plane's refund of this build's output window.
func (r *BuildRelay) Credit(c *agentpb.BuildCredit) {
	b := r.lookup(c.GetBuildId())
	if b == nil {
		return
	}
	select {
	case b.credit <- c.GetFrames():
	default:
		r.failAndStop(c.GetBuildId(), b, ErrBuildWindowViolated)
	}
}

// Cancel stops a build the control plane no longer wants. No terminal
// frame follows: the caller that cancelled is not reading anymore.
func (r *BuildRelay) Cancel(buildID string) {
	if b := r.discard(buildID); b != nil {
		b.stop()
	}
}

// CloseAll stops every in-flight build, for a session that is ending.
func (r *BuildRelay) CloseAll() {
	r.mu.Lock()
	live := r.live
	r.live = make(map[string]*agentBuild)
	r.mu.Unlock()

	for _, b := range live {
		b.stop()
	}
}

// fail reports err as this build's one terminal frame. A cancelled
// build's failure describes the cancel, not the build, and nobody is
// listening for it either way.
func (r *BuildRelay) fail(buildID string, b *agentBuild, err error) {
	if b.isCancelled() {
		return
	}
	b.sendTerminal(r.send, &agentpb.BuildOutput{
		BuildId: buildID,
		Frame:   &agentpb.BuildOutput_Failure{Failure: buildFailureToPB(err)},
	})
}

// failAndStop ends a build with err for a failure the build itself cannot
// observe (a peer overrunning its window).
func (r *BuildRelay) failAndStop(buildID string, b *agentBuild, err error) {
	if r.discard(buildID) == nil {
		return
	}
	r.fail(buildID, b, err)
	b.stop()
}

func (r *BuildRelay) sendResponse(buildID, errMsg string) {
	resp := &agentpb.AgentResponse{RequestId: buildID}
	if errMsg != "" {
		resp.Error = errMsg
	} else {
		resp.Result = emptyResult()
	}
	r.send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: resp}})
}

func (r *BuildRelay) lookup(buildID string) *agentBuild {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.live[buildID]
}

// discard removes buildID from the live set, returning it only to the
// first caller, so exactly one of them sends its terminal frame.
func (r *BuildRelay) discard(buildID string) *agentBuild {
	r.mu.Lock()
	defer r.mu.Unlock()
	b, ok := r.live[buildID]
	if !ok {
		return nil
	}
	delete(r.live, buildID)
	return b
}

// buildImageWriter is what BuildKit's exporter writes the finished image
// into: the docker-save tar, chunked into windowed frames, instead of
// this node's own image store.
type buildImageWriter struct {
	ctx     context.Context
	relay   *BuildRelay
	build   *agentBuild
	buildID string
}

func (w *buildImageWriter) Write(p []byte) (int, error) {
	written := 0
	for len(p) > 0 {
		n := min(len(p), buildChunkBytes)
		chunk := make([]byte, n)
		copy(chunk, p[:n])
		if err := w.build.sendData(w.ctx, w.relay.send, &agentpb.BuildOutput{
			BuildId: w.buildID,
			Frame:   &agentpb.BuildOutput_ImageChunk{ImageChunk: chunk},
		}); err != nil {
			return written, err
		}
		written += n
		p = p[n:]
	}
	return written, nil
}
