package migrateagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
	"github.com/GLINCKER/levelrail/internal/store"
)

func marshalJSON(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("migrateagent: encode: %w", err)
	}
	return string(raw), nil
}

// register makes c the live connection for id and closes any older one.
func (s *Service) register(id string, c *liveConn) {
	s.mu.Lock()
	old := s.live[id]
	s.live[id] = c
	s.mu.Unlock()
	if old != nil {
		old.kickWith("superseded")
	}
}

func (s *Service) unregister(id string, c *liveConn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.live[id] != c {
		return false
	}
	delete(s.live, id)
	return true
}

type inbound struct {
	msg *migrationpb.AgentMessage
	err error
}

// Session serves one source host's stream. It sends only the three migration
// control messages and answers nothing else.
func (s *Service) Session(stream migrationpb.MigrationService_SessionServer) error {
	ctx := stream.Context()
	sess, err := s.authenticate(ctx)
	if err != nil {
		return err
	}
	id := sess.ID
	c := newLiveConn()
	s.register(id, c)
	if cur, gerr := s.st.GetMigrationAgentSession(ctx, id); gerr != nil || cur.State == store.MigrationAgentRevoked {
		c.kickWith("revoked")
	}
	s.audit.Record(ctx, Event{Action: ActionSessionStarted, SessionID: id, Actor: "migration-agent", Remote: remoteAddr(ctx)})
	endReason := "stream closed"
	defer func() {
		close(c.dead)
		if s.unregister(id, c) {
			_ = s.st.SetMigrationAgentState(context.WithoutCancel(ctx), id, store.MigrationAgentLost, s.now())
		}
		s.audit.Record(ctx, Event{Action: ActionSessionEnded, SessionID: id, Actor: "migration-agent", Detail: map[string]any{"reason": endReason}})
	}()

	in := make(chan inbound)
	go func() {
		for {
			m, rerr := stream.Recv()
			select {
			case in <- inbound{m, rerr}:
			case <-ctx.Done():
				return
			}
			if rerr != nil {
				return
			}
		}
	}()

	expiry := time.NewTimer(time.Until(*sess.SessionExpiresAt))
	defer expiry.Stop()
	for {
		select {
		case m := <-in:
			if m.err != nil {
				if errors.Is(m.err, io.EOF) {
					return nil
				}
				endReason = "receive error"
				return m.err
			}
			if verr := s.handleAgentMessage(ctx, id, c, m.msg); verr != nil {
				endReason = "protocol violation"
				return verr
			}
		case out := <-c.out:
			if err := stream.Send(out); err != nil {
				endReason = "send error"
				return fmt.Errorf("migrateagent: send: %w", err)
			}
		case reason := <-c.kick:
			endReason = reason
			_ = stream.Send(&migrationpb.ControlMessage{Payload: &migrationpb.ControlMessage_GoAway{GoAway: &migrationpb.GoAway{Reason: reason}}})
			if reason == "superseded" {
				return status.Error(codes.Aborted, "session replaced by a newer connection")
			}
			return status.Error(codes.PermissionDenied, "migration session "+reason)
		case <-expiry.C:
			endReason = "session cap reached"
			_ = s.st.SetMigrationAgentState(ctx, id, store.MigrationAgentExpired, s.now())
			s.audit.Record(ctx, Event{Action: ActionSessionExpired, SessionID: id, Actor: "migration-agent"})
			_ = stream.Send(&migrationpb.ControlMessage{Payload: &migrationpb.ControlMessage_GoAway{GoAway: &migrationpb.GoAway{Reason: "session cap reached"}}})
			return status.Error(codes.PermissionDenied, "migration session expired")
		case <-ctx.Done():
			return nil
		}
	}
}

func (s *Service) handleAgentMessage(ctx context.Context, id string, c *liveConn, m *migrationpb.AgentMessage) error {
	switch p := m.GetPayload().(type) {
	case *migrationpb.AgentMessage_Hello:
		_ = s.st.TouchMigrationAgentSession(ctx, id, p.Hello.GetVersion(), p.Hello.GetOs(), p.Hello.GetArch(), s.now())
	case *migrationpb.AgentMessage_Heartbeat:
		_ = s.st.TouchMigrationAgentSession(ctx, id, "", "", "", s.now())
	case *migrationpb.AgentMessage_Inventory:
		c.deliver(p.Inventory.GetRequestId(), m)
	case *migrationpb.AgentMessage_ScanResult:
		c.deliver(p.ScanResult.GetRequestId(), m)
	case *migrationpb.AgentMessage_Denied:
		s.audit.Record(ctx, Event{Action: ActionAgentDenied, SessionID: id, Actor: "migration-agent", Status: 403,
			Detail: map[string]any{"reason": p.Denied.GetReason()}})
		c.deliver(p.Denied.GetRequestId(), m)
	default:
		s.audit.Record(ctx, Event{Action: ActionProtocolViolated, SessionID: id, Actor: "migration-agent", Status: 400})
		s.log.Warn("migrateagent: unknown agent message", slog.String("session_id", id))
		return status.Error(codes.InvalidArgument, "unknown message")
	}
	return nil
}
