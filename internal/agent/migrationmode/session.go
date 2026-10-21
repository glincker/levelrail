package migrationmode

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
)

// ErrGoAway ends a session the control plane closed on purpose.
var ErrGoAway = errors.New("migrationmode: control plane closed the session")

// ErrUnknownControl ends a session that received a message outside the migration set.
var ErrUnknownControl = errors.New("migrationmode: control message outside the migration set")

// Handler answers the three control messages a migration source understands.
type Handler struct {
	Docker      *Docker
	Scanner     *Scanner
	ScanTimeout time.Duration
	Now         func() time.Time
	Log         *slog.Logger
}

// Handle answers one control message. Any payload other than discover, scan or
// go_away, including one added to the proto later, is a protocol violation.
func (h *Handler) Handle(ctx context.Context, msg *migrationpb.ControlMessage) (*migrationpb.AgentMessage, error) {
	switch p := msg.GetPayload().(type) {
	case *migrationpb.ControlMessage_Discover:
		inv, err := Discover(ctx, h.Docker, p.Discover.GetRequestId(), h.Now())
		if err != nil {
			return &migrationpb.AgentMessage{Payload: &migrationpb.AgentMessage_Denied{Denied: &migrationpb.Denied{
				RequestId: p.Discover.GetRequestId(), Reason: err.Error()}}}, nil
		}
		h.Scanner.Learn(inv)
		return &migrationpb.AgentMessage{Payload: &migrationpb.AgentMessage_Inventory{Inventory: inv}}, nil
	case *migrationpb.ControlMessage_Scan:
		sctx, cancel := context.WithTimeout(ctx, h.ScanTimeout)
		defer cancel()
		res := h.Scanner.Scan(sctx, p.Scan.GetRequestId(), p.Scan.GetTargets())
		return &migrationpb.AgentMessage{Payload: &migrationpb.AgentMessage_ScanResult{ScanResult: res}}, nil
	case *migrationpb.ControlMessage_GoAway:
		return nil, fmt.Errorf("%w: %s", ErrGoAway, p.GoAway.GetReason())
	default:
		return nil, ErrUnknownControl
	}
}

// Stream is the Session RPC surface the loop needs.
type Stream interface {
	Send(*migrationpb.AgentMessage) error
	Recv() (*migrationpb.ControlMessage, error)
}

// Serve runs the session loop until the stream ends, the context is canceled
// or the control plane sends something outside the migration set.
func Serve(ctx context.Context, st Stream, h *Handler, version, goos, arch string, heartbeat time.Duration) error {
	var mu sync.Mutex
	send := func(m *migrationpb.AgentMessage) error {
		mu.Lock()
		defer mu.Unlock()
		return st.Send(m)
	}
	hello := &migrationpb.Hello{Version: version, Os: goos, Arch: arch}
	if err := send(&migrationpb.AgentMessage{Payload: &migrationpb.AgentMessage_Hello{Hello: hello}}); err != nil {
		return fmt.Errorf("migrationmode: send hello: %w", err)
	}
	done := make(chan struct{})
	defer close(done)
	go func() {
		t := time.NewTicker(heartbeat)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				if err := send(&migrationpb.AgentMessage{Payload: &migrationpb.AgentMessage_Heartbeat{Heartbeat: &migrationpb.Heartbeat{}}}); err != nil {
					return
				}
			}
		}
	}()
	for {
		msg, err := st.Recv()
		if err != nil {
			return fmt.Errorf("migrationmode: receive: %w", err)
		}
		reply, err := h.Handle(ctx, msg)
		if err != nil {
			return err
		}
		if err := send(reply); err != nil {
			return fmt.Errorf("migrationmode: send: %w", err)
		}
	}
}
