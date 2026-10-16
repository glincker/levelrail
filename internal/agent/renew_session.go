package agent

import (
	"context"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

// renewalStream carries the Renewer's session wiring: it serializes every
// Send (the report goroutine shares the stream with serveSession) and turns
// a RenewNow frame into a renewal instead of passing it on.
type renewalStream struct {
	inner agentClientStream
	r     *Renewer
	mu    sync.Mutex
}

func (s *renewalStream) Send(m *agentpb.AgentMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.inner.Send(m)
}

func (s *renewalStream) Recv() (*agentpb.ControlMessage, error) {
	for {
		m, err := s.inner.Recv()
		if err != nil {
			return nil, err
		}
		if m.GetRenewNow() != nil {
			s.r.RenewNow()
			continue
		}
		return m, nil
	}
}

// reportRenewals sends the renewer's status whenever it changes, until ctx
// is done.
func reportRenewals(ctx context.Context, r *Renewer, send func(*agentpb.AgentMessage) error) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-r.Updates():
			if err := send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Renewal{Renewal: renewalReport(r.Status())}}); err != nil {
				return
			}
		}
	}
}

func renewalReport(st RenewalStatus) *agentpb.RenewalReport {
	rep := &agentpb.RenewalReport{LastError: st.LastError, ConsecutiveFailures: int32(min(st.ConsecutiveFailures, 1<<30))} //nolint:gosec // clamped
	if !st.NextRenewAt.IsZero() {
		rep.NextRenewAt = timestamppb.New(st.NextRenewAt)
	}
	if !st.LastFailedAt.IsZero() {
		rep.LastFailedAt = timestamppb.New(st.LastFailedAt)
	}
	if !st.NextRetryAt.IsZero() {
		rep.NextRetryAt = timestamppb.New(st.NextRetryAt)
	}
	return rep
}
