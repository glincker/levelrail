package agent

// This file: the control plane's side of the request/
// response multiplexer over one Session stream. agent.proto's own
// header comment already explains why a single physical stream carries
// many logical request/response pairs; mux is what actually does that
// matching, keyed by AgentRequest.RequestId.

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

// sessionStream is the narrow surface mux needs from a Session gRPC
// stream, so tests can fake it without a real network connection.
// *grpc.GenericServerStream[AgentMessage, ControlMessage] (the control
// plane's own side of Session, per agentpb.AgentService_SessionServer)
// satisfies this structurally.
type sessionStream interface {
	Send(*agentpb.ControlMessage) error
	Recv() (*agentpb.AgentMessage, error)
}

// ErrSessionClosed is returned by mux.Call once the underlying stream
// has ended, for any pending or future call.
var ErrSessionClosed = errors.New("agent: session closed")

// eventChanBuffer bounds each watcher's event channel: recvLoop is the
// one goroutine reading every frame off the physical stream (responses
// and events alike), so it must never block delivering an event to a
// slow watcher, or every pending Call on the same stream would stall
// behind it too. A full channel drops the event rather than blocking;
// GRPCTransport.Events' own doc comment names this as a real,
// documented backpressure choice, not silently lossy behavior nobody
// decided on.
const eventChanBuffer = 64

// execChunkBytes bounds one exec data frame's payload (stdout or stdin).
const execChunkBytes = 32 << 10

// execWindowFrames is how many unacknowledged exec data frames a sender
// may have outstanding before it has to wait for an ExecCredit refund,
// bounding an in-flight exec at execWindowFrames*execChunkBytes (2 MiB)
// buffered per direction. Exec deliberately does not share
// eventChanBuffer's drop-on-full behavior above: a dropped event frame
// costs a missed reconcile trigger the next resync recovers from, while
// a dropped exec frame is a silently corrupted database dump. So exec
// makes the sender wait instead, and neither side may exceed its window.
const execWindowFrames = 64

// execCreditBatch is how many consumed frames accumulate before a
// receiver refunds them, halving credit-frame chatter while still
// refunding well before the sender's window runs dry.
const execCreditBatch = execWindowFrames / 2

// ErrExecWindowViolated ends an exec stream whose peer sent more
// unacknowledged data frames than execWindowFrames allows. Failing the
// one exec is the only safe response: blocking recvLoop would stall
// every other call sharing this session, and dropping the frame would
// corrupt the stream silently.
var ErrExecWindowViolated = errors.New("agent: exec peer exceeded its flow-control window")

// buildChunkBytes and buildWindowFrames are exec's windowing applied to a
// dispatched build, with a larger frame because a build moves a whole
// source tree up and a whole image tar back rather than a terminal's
// worth of output: 64 frames of 64 KiB bounds each direction at 4 MiB in
// flight.
const (
	buildChunkBytes   = 64 << 10
	buildWindowFrames = 64
	buildCreditBatch  = buildWindowFrames / 2
)

// ErrBuildWindowViolated ends a dispatched build whose peer sent more
// unacknowledged data frames than buildWindowFrames allows, for the same
// reason ErrExecWindowViolated ends an exec.
var ErrBuildWindowViolated = errors.New("agent: build peer exceeded its flow-control window")

// mux dispatches AgentRequest/AgentResponse pairs over one Session
// stream, from the control plane's side (the caller): Call sends a
// request and blocks until its matching response arrives, is cancelled
// via ctx, or the stream itself closes. Separately, it fans out
// unprompted ProxiedEvent frames to whichever watcher subscribed with
// the matching watch_id (subscribe/unsubscribe), the mechanism
// GRPCTransport.Events builds its returned channels on top of. A single
// background goroutine (recvLoop) owns the stream's Recv side, since a
// gRPC stream is not safe for concurrent Recv calls; Send is serialized
// separately for the identical reason on the write side.
type mux struct {
	stream sessionStream

	sendMu sync.Mutex

	mu       sync.Mutex
	pending  map[string]chan *agentpb.AgentResponse // nil once closed
	watchers map[string]chan *agentpb.ProxiedEvent  // nil once closed
	execs    map[string]*execSub                    // nil once closed
	builds   map[string]*buildSub                   // nil once closed
	err      error                                  // set once, right before the maps above are nilled

	closed chan struct{}
}

// frameSub is one in-flight multi-frame operation's control-plane-side
// delivery point: out carries the agent's data frames, credit carries
// the agent's refunds of this side's own input window. Only recvLoop
// ever closes either channel (it is the only sender on both), so a
// caller abandoning the operation unsubscribes without closing and lets
// its own done signal wake any blocked reader instead.
type frameSub[T any] struct {
	out    chan T
	credit chan uint32

	mu  sync.Mutex
	err error
}

func (s *frameSub[T]) fail(err error) {
	s.mu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.mu.Unlock()
}

func (s *frameSub[T]) failure() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

type (
	execSub  = frameSub[*agentpb.ExecOutput]
	buildSub = frameSub[*agentpb.BuildOutput]
)

// newMux starts dispatching stream immediately (recvLoop runs in its
// own goroutine from this call onward).
func newMux(stream sessionStream) *mux {
	m := &mux{
		stream:   stream,
		pending:  make(map[string]chan *agentpb.AgentResponse),
		watchers: make(map[string]chan *agentpb.ProxiedEvent),
		execs:    make(map[string]*execSub),
		builds:   make(map[string]*buildSub),
		closed:   make(chan struct{}),
	}
	go m.recvLoop()
	return m
}

func (m *mux) recvLoop() {
	for {
		msg, err := m.stream.Recv()
		if err != nil {
			m.shutdown(err)
			return
		}
		switch p := msg.GetPayload().(type) {
		case *agentpb.AgentMessage_Response:
			m.deliver(p.Response)
		case *agentpb.AgentMessage_Event:
			m.deliverEvent(p.Event)
		case *agentpb.AgentMessage_ExecOutput:
			m.deliverExecOutput(p.ExecOutput)
		case *agentpb.AgentMessage_ExecCredit:
			m.deliverExecCredit(p.ExecCredit)
		case *agentpb.AgentMessage_BuildOutput:
			m.deliverBuildOutput(p.BuildOutput)
		case *agentpb.AgentMessage_BuildCredit:
			m.deliverBuildCredit(p.BuildCredit)
		}
	}
}

func (m *mux) deliverEvent(ev *agentpb.ProxiedEvent) {
	m.mu.Lock()
	ch, ok := m.watchers[ev.GetWatchId()]
	m.mu.Unlock()
	if !ok {
		// No subscriber for this watch_id (never subscribed, or already
		// unsubscribed): dropped, the same "nobody's waiting" handling
		// deliver already applies to a stray AgentResponse.
		return
	}
	select {
	case ch <- ev:
	default:
		// Full buffer: drop rather than block recvLoop, per
		// eventChanBuffer's own doc comment.
	}
}

// subscribe registers a fresh, buffered channel for watchID, replacing
// any prior subscription under the same ID (there should never be one:
// each GRPCTransport.Events call mints its own watchID). Returns nil if
// the mux is already closed, so a caller that races Events() against
// the stream ending gets a clear signal instead of a channel that will
// simply never receive anything.
func (m *mux) subscribe(watchID string) chan *agentpb.ProxiedEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.watchers == nil {
		return nil
	}
	ch := make(chan *agentpb.ProxiedEvent, eventChanBuffer)
	m.watchers[watchID] = ch
	return ch
}

// unsubscribe removes and closes watchID's event channel, if it still
// exists (a mux shutdown may have already closed and removed it).
func (m *mux) unsubscribe(watchID string) {
	m.mu.Lock()
	ch, ok := m.watchers[watchID]
	if ok {
		delete(m.watchers, watchID)
	}
	m.mu.Unlock()
	if ok {
		close(ch)
	}
}

// subscribeExec registers delivery channels for execID (which is the
// exec's own AgentRequest.RequestId). Returns nil if the mux is already
// closed. Both channels are sized a frame past the flow-control window
// so a well-behaved agent's data frames plus its one terminal frame can
// always be delivered without recvLoop blocking.
func (m *mux) subscribeExec(execID string) *execSub {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.execs == nil {
		return nil
	}
	sub := &execSub{
		out:    make(chan *agentpb.ExecOutput, execWindowFrames+1),
		credit: make(chan uint32, execWindowFrames+1),
	}
	m.execs[execID] = sub
	return sub
}

// unsubscribeExec stops delivering execID's frames. It deliberately does
// not close the subscription's channels: recvLoop is their only sender
// and may be mid-delivery right now, so closing here would race into a
// send on a closed channel. The abandoning caller wakes its own reader.
func (m *mux) unsubscribeExec(execID string) {
	m.mu.Lock()
	delete(m.execs, execID)
	m.mu.Unlock()
}

func (m *mux) execSubscription(execID string) *execSub {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.execs[execID]
}

func (m *mux) deliverExecOutput(out *agentpb.ExecOutput) {
	sub := m.execSubscription(out.GetExecId())
	if sub == nil {
		return
	}
	select {
	case sub.out <- out:
	default:
		m.failExec(out.GetExecId(), ErrExecWindowViolated)
	}
}

func (m *mux) deliverExecCredit(credit *agentpb.ExecCredit) {
	sub := m.execSubscription(credit.GetExecId())
	if sub == nil {
		return
	}
	select {
	case sub.credit <- credit.GetFrames():
	default:
		m.failExec(credit.GetExecId(), ErrExecWindowViolated)
	}
}

// failExec ends execID's stream with err. Only ever called from
// recvLoop, which is why closing the channels here is safe.
func (m *mux) failExec(execID string, err error) {
	m.mu.Lock()
	sub, ok := m.execs[execID]
	if ok {
		delete(m.execs, execID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	sub.fail(err)
	close(sub.out)
	close(sub.credit)
}

// subscribeBuild registers delivery channels for buildID (which is the
// build's own AgentRequest.RequestId), the exact shape subscribeExec
// establishes for an exec, sized a frame past the flow-control window so
// a well-behaved agent's data frames plus its one terminal frame can
// always be delivered without recvLoop blocking.
func (m *mux) subscribeBuild(buildID string) *buildSub {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.builds == nil {
		return nil
	}
	sub := &buildSub{
		out:    make(chan *agentpb.BuildOutput, buildWindowFrames+1),
		credit: make(chan uint32, buildWindowFrames+1),
	}
	m.builds[buildID] = sub
	return sub
}

// unsubscribeBuild stops delivering buildID's frames, deliberately
// without closing its channels: see unsubscribeExec for why.
func (m *mux) unsubscribeBuild(buildID string) {
	m.mu.Lock()
	delete(m.builds, buildID)
	m.mu.Unlock()
}

func (m *mux) buildSubscription(buildID string) *buildSub {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.builds[buildID]
}

func (m *mux) deliverBuildOutput(out *agentpb.BuildOutput) {
	sub := m.buildSubscription(out.GetBuildId())
	if sub == nil {
		return
	}
	select {
	case sub.out <- out:
	default:
		m.failBuild(out.GetBuildId(), ErrBuildWindowViolated)
	}
}

func (m *mux) deliverBuildCredit(credit *agentpb.BuildCredit) {
	sub := m.buildSubscription(credit.GetBuildId())
	if sub == nil {
		return
	}
	select {
	case sub.credit <- credit.GetFrames():
	default:
		m.failBuild(credit.GetBuildId(), ErrBuildWindowViolated)
	}
}

// failBuild ends buildID's stream with err. Only ever called from
// recvLoop, which is why closing the channels here is safe.
func (m *mux) failBuild(buildID string, err error) {
	m.mu.Lock()
	sub, ok := m.builds[buildID]
	if ok {
		delete(m.builds, buildID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	sub.fail(err)
	close(sub.out)
	close(sub.credit)
}

func (m *mux) sendBuildInput(in *agentpb.BuildInput) error {
	return m.sendFrame(&agentpb.ControlMessage{Payload: &agentpb.ControlMessage_BuildInput{BuildInput: in}})
}

func (m *mux) sendBuildCancel(buildID string) error {
	return m.sendFrame(&agentpb.ControlMessage{Payload: &agentpb.ControlMessage_BuildCancel{
		BuildCancel: &agentpb.BuildCancel{BuildId: buildID},
	}})
}

func (m *mux) sendBuildCredit(buildID string, frames uint32) error {
	return m.sendFrame(&agentpb.ControlMessage{Payload: &agentpb.ControlMessage_BuildCredit{
		BuildCredit: &agentpb.BuildCredit{BuildId: buildID, Frames: frames},
	}})
}

// sendFrame writes one ControlMessage down the stream, serialized
// against every other writer (a gRPC stream is not safe for concurrent
// Send calls).
func (m *mux) sendFrame(msg *agentpb.ControlMessage) error {
	m.sendMu.Lock()
	defer m.sendMu.Unlock()
	if err := m.stream.Send(msg); err != nil {
		return fmt.Errorf("agent: send frame: %w", err)
	}
	return nil
}

func (m *mux) sendExecInput(in *agentpb.ExecInput) error {
	return m.sendFrame(&agentpb.ControlMessage{Payload: &agentpb.ControlMessage_ExecInput{ExecInput: in}})
}

func (m *mux) sendExecCancel(execID string) error {
	return m.sendFrame(&agentpb.ControlMessage{Payload: &agentpb.ControlMessage_ExecCancel{
		ExecCancel: &agentpb.ExecCancel{ExecId: execID},
	}})
}

func (m *mux) sendExecCredit(execID string, frames uint32) error {
	return m.sendFrame(&agentpb.ControlMessage{Payload: &agentpb.ControlMessage_ExecCredit{
		ExecCredit: &agentpb.ExecCredit{ExecId: execID, Frames: frames},
	}})
}

func (m *mux) deliver(resp *agentpb.AgentResponse) {
	m.mu.Lock()
	ch, ok := m.pending[resp.GetRequestId()]
	if ok {
		delete(m.pending, resp.GetRequestId())
	}
	m.mu.Unlock()
	if ok {
		ch <- resp
	}
	// An unknown request_id (no ok) is a response for a call this mux
	// already gave up waiting on (ctx cancelled, most likely): dropped,
	// not an error, since there is no one left to deliver it to.
}

// shutdown ends every pending Call and every active event subscription
// with err, and marks the mux closed for any future Call or subscribe.
// Idempotent: only the first caller (recvLoop itself, always, since
// it's the only caller) actually does anything.
func (m *mux) shutdown(err error) {
	m.mu.Lock()
	if m.pending == nil {
		m.mu.Unlock()
		return
	}
	pending := m.pending
	watchers := m.watchers
	execs := m.execs
	builds := m.builds
	m.pending = nil
	m.watchers = nil
	m.execs = nil
	m.builds = nil
	m.err = err
	m.mu.Unlock()

	for _, ch := range pending {
		close(ch)
	}
	for _, ch := range watchers {
		close(ch)
	}
	for _, sub := range execs {
		sub.fail(fmt.Errorf("%w: %v", ErrSessionClosed, err))
		close(sub.out)
		close(sub.credit)
	}
	for _, sub := range builds {
		sub.fail(fmt.Errorf("%w: %v", ErrSessionClosed, err))
		close(sub.out)
		close(sub.credit)
	}
	close(m.closed)
}

// Call sends req (assigning it a fresh RequestId, overwriting whatever
// was there) down the stream and waits for the matching AgentResponse.
// A response carrying AgentResponse.Error becomes a plain Go error here
// (errors.New, not %w-wrapped: see execute.go's own doc comment on why
// there's nothing meaningful to wrap across this RPC boundary).
func (m *mux) Call(ctx context.Context, req *agentpb.AgentRequest) (*agentpb.AgentResponse, error) {
	id, err := randomRequestID()
	if err != nil {
		return nil, err
	}
	return m.CallWithID(ctx, id, req)
}

// CallWithID is Call with a caller-chosen RequestId, for an operation
// whose ID has to exist before the request is sent: an exec subscribes
// to its own output under that same ID first, so the agent's first
// output frame can never arrive with nowhere to go.
func (m *mux) CallWithID(ctx context.Context, id string, req *agentpb.AgentRequest) (*agentpb.AgentResponse, error) {
	req.RequestId = id

	ch := make(chan *agentpb.AgentResponse, 1)
	m.mu.Lock()
	if m.pending == nil {
		sessionErr := m.err
		m.mu.Unlock()
		return nil, fmt.Errorf("%w: %v", ErrSessionClosed, sessionErr)
	}
	m.pending[id] = ch
	m.mu.Unlock()

	if sendErr := m.sendFrame(&agentpb.ControlMessage{Payload: &agentpb.ControlMessage_Request{Request: req}}); sendErr != nil {
		m.mu.Lock()
		delete(m.pending, id)
		m.mu.Unlock()
		return nil, sendErr
	}

	select {
	case resp, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("%w: %v", ErrSessionClosed, m.err)
		}
		if resp.GetError() != "" {
			return nil, errors.New(resp.GetError())
		}
		return resp, nil
	case <-ctx.Done():
		m.mu.Lock()
		delete(m.pending, id)
		m.mu.Unlock()
		return nil, ctx.Err()
	case <-m.closed:
		return nil, fmt.Errorf("%w: %v", ErrSessionClosed, m.err)
	}
}

func randomRequestID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("agent: generate request id: %w", err)
	}
	return hex.EncodeToString(buf), nil
}
