package agent

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/agent/agentpb"
)

func removeReq(token uint64, target string) *agentpb.AgentRequest {
	return &agentpb.AgentRequest{Op: &agentpb.AgentRequest_Remove{Remove: &agentpb.RemoveRequest{Id: "c1", FenceToken: token, FenceTarget: target}}}
}

func TestFenceBookAdmit(t *testing.T) {
	b := NewFenceBook()
	steps := []struct {
		name    string
		token   uint64
		wantErr bool
	}{
		{"first token accepted", 3, false},
		{"equal token is an idempotent replay", 3, false},
		{"higher token accepted", 5, false},
		{"stale token rejected", 4, true},
		{"unfenced request passes", 0, false},
	}
	for _, s := range steps {
		err := b.CheckRequest(removeReq(s.token, "web"))
		if (err != nil) != s.wantErr {
			t.Errorf("%s: err = %v, wantErr %v", s.name, err, s.wantErr)
		}
	}
	if err := b.CheckRequest(removeReq(1, "other")); err != nil {
		t.Errorf("a different target keeps its own counter: %v", err)
	}
	if err := b.CheckRequest(&agentpb.AgentRequest{Op: &agentpb.AgentRequest_Stop{Stop: &agentpb.StopRequest{Id: "c", FenceToken: 2, FenceTarget: "web"}}}); err == nil {
		t.Error("a stale stop must be refused too")
	}
}

func TestFenceBookPersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fence.json")
	a := NewFenceBook()
	a.SetPath(path)
	if err := a.Admit("web", 7); err != nil {
		t.Fatal(err)
	}
	b := NewFenceBook()
	b.SetPath(path)
	if err := b.Admit("web", 6); err == nil {
		t.Fatal("a restarted agent must still refuse a stale token")
	}
}

func TestFenceStampOldAgent(t *testing.T) {
	for _, required := range []bool{false, true} {
		tr := newGRPCTransport(newMux(newFakeSessionStream()))
		tr.fence = FenceHooks{Required: func(context.Context) bool { return required }}
		tok, target, err := tr.fenceStamp(WithFence(context.Background(), "web", 4))
		if required {
			if !errors.Is(err, ErrFenceUnsupported) {
				t.Errorf("required on: err = %v, want ErrFenceUnsupported", err)
			}
			continue
		}
		if err != nil || tok != 0 || target != "" {
			t.Errorf("required off: tok=%d target=%q err=%v, want an unfenced send", tok, target, err)
		}
	}
}

func TestDestructiveRejectsStaleCompletion(t *testing.T) {
	cases := []struct {
		name    string
		current uint64
		wantErr error
	}{
		{"equal token completes", 4, nil},
		{"newer record is reported, not failed", 5, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stream := newFakeSessionStream()
			caps := &capSet{}
			caps.store([]string{CapFenceToken})
			tr := newGRPCTransportWithCaps(newMux(stream), caps)
			var rejected int
			tr.fence = FenceHooks{
				Required: func(context.Context) bool { return true },
				Current:  func(context.Context, string) (uint64, error) { return c.current, nil },
				Rejected: func(context.Context, string, string, uint64, uint64, string) { rejected++ },
			}
			done := make(chan error, 1)
			go func() { done <- tr.Remove(WithFence(context.Background(), "web", 4), "c1", true) }()
			var req *agentpb.AgentRequest
			select {
			case msg := <-stream.sent:
				req = msg.GetRequest()
			case <-time.After(2 * time.Second):
				t.Fatal("no request sent")
			}
			if got := req.GetRemove(); got.GetFenceToken() != 4 || got.GetFenceTarget() != "web" {
				t.Fatalf("request carries token %d target %q", got.GetFenceToken(), got.GetFenceTarget())
			}
			stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: &agentpb.AgentResponse{RequestId: req.GetRequestId()}}}
			select {
			case err := <-done:
				if !errors.Is(err, c.wantErr) {
					t.Fatalf("err = %v, want %v", err, c.wantErr)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("call did not return")
			}
			if (c.current > 4) != (rejected == 1) {
				t.Errorf("rejected events = %d", rejected)
			}
		})
	}
}

func TestFenceCtxAroundMove(t *testing.T) {
	token := uint64(1)
	read := func(context.Context, string) (uint64, error) { return token, nil }
	caps := &capSet{}
	caps.store([]string{CapFenceToken})

	type sent struct {
		tok    uint64
		target string
		err    error
	}
	run := func(ctx context.Context, current uint64) sent {
		stream := newFakeSessionStream()
		tr := newGRPCTransportWithCaps(newMux(stream), caps)
		tr.fence = FenceHooks{Required: func(context.Context) bool { return true }, Current: func(context.Context, string) (uint64, error) { return current, nil }}
		done := make(chan error, 1)
		go func() { done <- tr.Remove(ctx, "c1", true) }()
		var req *agentpb.AgentRequest
		select {
		case msg := <-stream.sent:
			req = msg.GetRequest()
		case <-time.After(2 * time.Second):
			t.Fatal("no request sent")
		}
		stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: &agentpb.AgentResponse{RequestId: req.GetRequestId()}}}
		var err error
		select {
		case err = <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("call did not return")
		}
		return sent{req.GetRemove().GetFenceToken(), req.GetRemove().GetFenceTarget(), err}
	}

	before := FenceCtx(context.Background(), read, "web")
	token = 2 // a move bumps the placement token
	after := FenceCtx(context.Background(), read, "web")

	cases := []struct {
		name    string
		ctx     context.Context
		wantTok uint64
		wantErr error
	}{
		{"action after the move carries the new token", after, 2, nil},
		{"in-flight action from before the move is superseded", before, 1, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := run(c.ctx, token)
			if got.tok != c.wantTok || got.target != "web" || !errors.Is(got.err, c.wantErr) {
				t.Fatalf("token=%d target=%q err=%v, want token %d err %v", got.tok, got.target, got.err, c.wantTok, c.wantErr)
			}
		})
	}
	unfenced := FenceCtx(context.Background(), func(context.Context, string) (uint64, error) { return 0, nil }, "web")
	if _, ok := fenceFrom(unfenced); ok {
		t.Error("a zero token must leave the context unfenced")
	}
}

func TestFenceStampSendsNoTokenWhileSettingOff(t *testing.T) {
	cases := []struct {
		name     string
		required func(context.Context) bool
		wantTok  uint64
	}{
		{"setting off", func(context.Context) bool { return false }, 0},
		{"no hook", nil, 0},
		{"setting on", func(context.Context) bool { return true }, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := &capSet{}
			caps.store([]string{CapFenceToken})
			tr := newGRPCTransportWithCaps(newMux(newFakeSessionStream()), caps)
			tr.fence = FenceHooks{Required: c.required}
			tok, _, err := tr.fenceStamp(WithFence(context.Background(), "web", 7))
			if err != nil || tok != c.wantTok {
				t.Fatalf("tok=%d err=%v, want %d", tok, err, c.wantTok)
			}
		})
	}
}

func TestFenceBookAdmitsTokenZeroAlways(t *testing.T) {
	b := NewFenceBook()
	if err := b.Admit("web", 9); err != nil {
		t.Fatal(err)
	}
	if err := b.Admit("web", 0); err != nil {
		t.Fatalf("token 0 must pass: %v", err)
	}
	if err := b.Admit("web", 3); err == nil {
		t.Fatal("a lower token must be refused")
	}
}
