package agent

// This file: the control plane's side of TASKS.md 3.2's request/
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
	err      error                                  // set once, right before pending/watchers are nilled

	closed chan struct{}
}

// newMux starts dispatching stream immediately (recvLoop runs in its
// own goroutine from this call onward).
func newMux(stream sessionStream) *mux {
	m := &mux{
		stream:   stream,
		pending:  make(map[string]chan *agentpb.AgentResponse),
		watchers: make(map[string]chan *agentpb.ProxiedEvent),
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
	m.pending = nil
	m.watchers = nil
	m.err = err
	m.mu.Unlock()

	for _, ch := range pending {
		close(ch)
	}
	for _, ch := range watchers {
		close(ch)
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

	m.sendMu.Lock()
	sendErr := m.stream.Send(&agentpb.ControlMessage{Request: req})
	m.sendMu.Unlock()
	if sendErr != nil {
		m.mu.Lock()
		delete(m.pending, id)
		m.mu.Unlock()
		return nil, fmt.Errorf("agent: send request: %w", sendErr)
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
