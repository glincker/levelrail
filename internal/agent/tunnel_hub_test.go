package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"runtime"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/glincker/levelrail/internal/agent/agentpb"
	"github.com/glincker/levelrail/internal/experimental"
)

const testTunnelNode = "node-1"

type tunnelRig struct {
	hub    *tunnelHub
	tp     *GRPCTransport
	srv    *grpc.Server
	cc     *grpc.ClientConn
	cancel context.CancelFunc
	hbSeen chan time.Time

	mu       sync.Mutex
	behavior func(req *agentpb.TunnelOpenRequest) (refuse string, stall bool)
	opens    int
	// realAgent makes the test serve the data side itself instead of the echo stub.
	realAgent bool

	sendHeartbeat func()
}

type rigService struct {
	agentpb.UnimplementedAgentServiceServer
	r      *tunnelRig
	ready  chan struct{}
	caps   []string
	sessWG sync.WaitGroup
}

func (s *rigService) Session(stream agentpb.AgentService_SessionServer) error {
	caps := &capSet{}
	caps.store(s.caps)
	m := newMuxWithHandlers(stream, muxHandlers{heartbeat: func() {
		select {
		case s.r.hbSeen <- time.Now():
		default:
		}
	}})
	tp := newGRPCTransportWithCaps(m, caps)
	tp.attachTunnels(s.r.hub, testTunnelNode)
	s.r.tp = tp
	close(s.ready)
	<-m.closed
	s.r.hub.closeOwner(m)
	return nil
}

func (s *rigService) Tunnel(stream agentpb.AgentService_TunnelServer) error {
	return s.r.hub.serveTunnel(stream.Context(), testTunnelNode, stream)
}

func enableTunnelFlag(t *testing.T) {
	t.Helper()
	t.Setenv(experimental.EnvVar, string(experimental.AgentIngressTunnel))
	experimental.Reset()
	t.Cleanup(experimental.Reset)
}

// newTunnelRig runs a real gRPC server and a fake agent over bufconn, so tunnel and Session share one HTTP/2 connection.
func newTunnelRig(t *testing.T, caps []string, behavior func(*agentpb.TunnelOpenRequest) (string, bool)) *tunnelRig {
	t.Helper()
	enableTunnelFlag(t)
	lis := bufconn.Listen(1 << 20)
	r := &tunnelRig{hub: newTunnelHub(slog.New(slog.DiscardHandler)), hbSeen: make(chan time.Time, 64), behavior: behavior}
	svc := &rigService{r: r, ready: make(chan struct{}), caps: caps}
	r.srv = grpc.NewServer()
	agentpb.RegisterAgentServiceServer(r.srv, svc)
	go func() { _ = r.srv.Serve(lis) }()
	cc, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	r.cc = cc
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	client := agentpb.NewAgentServiceClient(cc)
	sess, err := client.Session(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var sendMu sync.Mutex
	send := func(m *agentpb.AgentMessage) {
		sendMu.Lock()
		defer sendMu.Unlock()
		_ = sess.Send(m)
	}
	go func() {
		for {
			msg, err := sess.Recv()
			if err != nil {
				return
			}
			req := msg.GetRequest()
			if req == nil {
				continue
			}
			resp := &agentpb.AgentResponse{RequestId: req.GetRequestId(), Result: &agentpb.AgentResponse_Empty{Empty: &agentpb.Empty{}}}
			if op := req.GetTunnelOpen(); op != nil {
				r.mu.Lock()
				r.opens++
				r.mu.Unlock()
				refuse, stall := r.behavior(op)
				resp = &agentpb.AgentResponse{RequestId: req.GetRequestId(), Result: &agentpb.AgentResponse_TunnelOpen{TunnelOpen: &agentpb.TunnelOpenResponse{FailureReason: refuse}}}
				if refuse == "" && !r.realAgent {
					go r.agentTunnel(ctx, client, op.GetTunnelId(), stall)
				}
			}
			send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: resp}})
		}
	}()
	r.sendHeartbeat = func() {
		send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Heartbeat{Heartbeat: &agentpb.Heartbeat{}}})
	}
	select {
	case <-svc.ready:
	case <-time.After(3 * time.Second):
		t.Fatal("session did not start")
	}
	t.Cleanup(func() {
		cancel()
		_ = cc.Close()
		r.srv.Stop()
	})
	return r
}

// agentTunnel is the fake agent's data side: an echo, or a peer that never reads.
func (r *tunnelRig) agentTunnel(ctx context.Context, c agentpb.AgentServiceClient, id []byte, stall bool) {
	st, err := c.Tunnel(ctx)
	if err != nil {
		return
	}
	if st.Send(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Hello{Hello: &agentpb.TunnelHello{TunnelId: id}}}) != nil {
		return
	}
	if stall {
		<-ctx.Done()
		return
	}
	for {
		f, err := st.Recv()
		if err != nil {
			return
		}
		switch fr := f.GetFrame().(type) {
		case *agentpb.TunnelFrame_Data:
			_ = st.Send(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Data{Data: fr.Data}})
		case *agentpb.TunnelFrame_HalfClose:
			_ = st.Send(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_HalfClose{HalfClose: &agentpb.Empty{}}})
			return
		}
	}
}

func echoOK(*agentpb.TunnelOpenRequest) (string, bool) { return "", false }

func TestTunnelEchoAndHalfClose(t *testing.T) {
	r := newTunnelRig(t, []string{CapIngressTunnel}, echoOK)
	c, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 8080, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	payload := bytes.Repeat([]byte("abcdefgh"), 20000) // 160 KB, several frames
	go func() {
		_, _ = c.Write(payload)
		_ = c.CloseWrite()
	}()
	got, err := io.ReadAll(c)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("echo mismatch: got %d bytes", len(got))
	}
	if _, err := c.Write([]byte("x")); err == nil {
		t.Fatal("write after CloseWrite must fail")
	}
	st := r.hub.stats()
	if st.OpenResults["ok"] != 1 || st.BytesToAgent != uint64(len(payload)) {
		t.Fatalf("stats = %+v", st)
	}
}

func TestTunnelRefusedWithReason(t *testing.T) {
	r := newTunnelRig(t, []string{CapIngressTunnel}, func(*agentpb.TunnelOpenRequest) (string, bool) { return "no running container", false })
	_, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 8080, time.Time{})
	var refused *TunnelRefusedError
	if !errors.Is(err, ErrTunnelRefused) || !errors.As(err, &refused) || refused.Reason != "no running container" {
		t.Fatalf("err = %v", err)
	}
	if n := len(r.hub.perNode); n != 0 {
		t.Fatalf("slot leaked after refusal: %d", n)
	}
}

func TestTunnelOldAgentAndFlagOff(t *testing.T) {
	r := newTunnelRig(t, nil, echoOK)
	_, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 8080, time.Time{})
	if !errors.Is(err, ErrTunnelUnsupported) || !errors.Is(err, ErrAgentLacksCapability) {
		t.Fatalf("no capability err = %v", err)
	}
	if r.opens != 0 {
		t.Fatal("tunnel_open sent to an agent without the capability")
	}
	t.Setenv(experimental.EnvVar, "")
	experimental.Reset()
	r2 := &GRPCTransport{caps: &capSet{}, tunnels: r.hub, nodeID: testTunnelNode}
	r2.caps.store([]string{CapIngressTunnel})
	if _, err := r2.OpenTunnel(context.Background(), testTunnelNode, "web", 8080, time.Time{}); !errors.Is(err, ErrTunnelUnsupported) {
		t.Fatalf("flag off err = %v", err)
	}
}

func TestTunnelNodeOffline(t *testing.T) {
	enableTunnelFlag(t)
	srv := &Server{registry: NewRegistry(), tunnels: newTunnelHub(slog.New(slog.DiscardHandler))}
	if _, err := srv.OpenTunnel(context.Background(), "ghost", "web", 80, time.Time{}); !errors.Is(err, ErrNodeOffline) {
		t.Fatalf("err = %v", err)
	}
	r := newTunnelRig(t, []string{CapIngressTunnel}, echoOK)
	r.cancel()
	deadline := time.Now().Add(3 * time.Second)
	for {
		_, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, time.Time{})
		if errors.Is(err, ErrNodeOffline) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("session end not reported, last err = %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTunnelPerNodeLimit(t *testing.T) {
	r := newTunnelRig(t, []string{CapIngressTunnel}, echoOK)
	r.hub.setLimits(TunnelLimits{MaxPerNode: 2, MaxPerService: 2})
	var open []*TunnelConn
	for i := 0; i < 2; i++ {
		c, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		open = append(open, c)
	}
	if _, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, time.Time{}); !errors.Is(err, ErrTunnelLimit) {
		t.Fatalf("third open err = %v, want ErrTunnelLimit", err)
	}
	_ = open[0].Close()
	c, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, time.Time{})
	if err != nil {
		t.Fatalf("open after a close: %v", err)
	}
	_ = c.Close()
	_ = open[1].Close()
}

func TestTunnelOpenTimeoutAndLifetime(t *testing.T) {
	// Absolute deadline first, then a handshake that gets no answer.
	r := newTunnelRig(t, []string{CapIngressTunnel}, func(*agentpb.TunnelOpenRequest) (string, bool) { return "", true })
	r.hub.setLimits(TunnelLimits{OpenTimeout: 200 * time.Millisecond, IdleTimeout: time.Minute})
	c, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, time.Now().Add(150*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("absolute deadline did not close the tunnel")
	}
	if _, err := c.Read(make([]byte, 1)); !errors.Is(err, ErrTunnelExpired) {
		t.Fatalf("read err = %v", err)
	}

	// No agent answer at all: the handshake times out.
	h := newTunnelHub(slog.New(slog.DiscardHandler))
	h.setLimits(TunnelLimits{OpenTimeout: 100 * time.Millisecond})
	m := newMux(newFakeSessionStream())
	tp := newGRPCTransportWithCaps(m, &capSet{})
	tp.caps.store([]string{CapIngressTunnel})
	tp.attachTunnels(h, testTunnelNode)
	if _, err := tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, time.Time{}); !errors.Is(err, ErrTunnelTimeout) {
		t.Fatalf("err = %v, want ErrTunnelTimeout", err)
	}
	if len(h.perNode) != 0 || len(h.pending) != 0 {
		t.Fatal("handshake timeout leaked a slot or pending entry")
	}
}

func TestTunnelIdleTimeout(t *testing.T) {
	r := newTunnelRig(t, []string{CapIngressTunnel}, echoOK)
	r.hub.setLimits(TunnelLimits{IdleTimeout: 150 * time.Millisecond})
	c, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("idle tunnel was not closed")
	}
	if _, err := c.Read(make([]byte, 1)); !errors.Is(err, ErrTunnelIdle) {
		t.Fatalf("read err = %v", err)
	}
}

func TestTunnelSaturatedDoesNotDelaySession(t *testing.T) {
	r := newTunnelRig(t, []string{CapIngressTunnel}, func(*agentpb.TunnelOpenRequest) (string, bool) { return "", true })
	c, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	// The agent never reads, so this write blocks on flow control once the window fills.
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		_, _ = c.Write(make([]byte, 64<<20))
	}()
	time.Sleep(200 * time.Millisecond)
	select {
	case <-writeDone:
		t.Fatal("tunnel write finished; the tunnel was not saturated")
	default:
	}

	start := time.Now()
	r.sendHeartbeat()
	select {
	case seen := <-r.hbSeen:
		if d := seen.Sub(start); d > time.Second {
			t.Fatalf("heartbeat delayed %v by a saturated tunnel", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat not delivered while a tunnel is saturated")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := r.tp.mux.Call(ctx, &agentpb.AgentRequest{Op: &agentpb.AgentRequest_Start{Start: &agentpb.StartRequest{Id: "c1"}}}); err != nil {
		t.Fatalf("small request on the same session failed: %v", err)
	}
	_ = c.Close()
	select {
	case <-writeDone:
	case <-time.After(3 * time.Second):
		t.Fatal("blocked write not released by Close")
	}
}

func TestTunnelCancelLeavesNoGoroutines(t *testing.T) {
	before := runtime.NumGoroutine()
	func() {
		r := newTunnelRig(t, []string{CapIngressTunnel}, echoOK)
		for i := 0; i < 5; i++ {
			c, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, time.Time{})
			if err != nil {
				t.Fatal(err)
			}
			_, _ = c.Write([]byte("hi"))
			if i%2 == 0 {
				_ = c.Close()
			}
		}
		r.cancel()
		_ = r.cc.Close()
		r.srv.Stop()
		deadline := time.Now().Add(3 * time.Second)
		for len(r.hub.stats().ActiveByNode) > 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if st := r.hub.stats(); st.ActiveTunnels != 0 || len(r.hub.perNode) != 0 {
			t.Fatalf("tunnels left after session end: %+v", st)
		}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before+2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+2 {
		buf := make([]byte, 1<<16)
		t.Fatalf("goroutines %d -> %d\n%s", before, n, buf[:runtime.Stack(buf, true)])
	}
}

func TestTunnelHelloRules(t *testing.T) {
	enableTunnelFlag(t)
	h := newTunnelHub(slog.New(slog.DiscardHandler))
	id, _ := newTunnelID()
	h.pending[string(id)] = &pendingTunnel{nodeID: "other", service: "web", ready: make(chan *TunnelConn, 1)}
	st := &fakeServerTunnel{in: make(chan *agentpb.TunnelFrame, 2), ctx: context.Background()}
	st.in <- &agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Hello{Hello: &agentpb.TunnelHello{TunnelId: id}}}
	if err := h.serveTunnel(st.ctx, testTunnelNode, st); err == nil {
		t.Fatal("hello from a different node must be refused")
	}
	if _, ok := h.pending[string(id)]; !ok {
		t.Fatal("a mismatched node must not consume the pending open")
	}
	st.in <- &agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Hello{Hello: &agentpb.TunnelHello{TunnelId: make([]byte, TunnelIDLen)}}}
	if err := h.serveTunnel(st.ctx, testTunnelNode, st); err == nil {
		t.Fatal("unknown id must be refused")
	}
	st.in <- &agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Data{Data: []byte("x")}}
	if err := h.serveTunnel(st.ctx, testTunnelNode, st); err == nil {
		t.Fatal("a first frame that is not hello must be refused")
	}
}

type fakeServerTunnel struct {
	grpc.ServerStream
	in  chan *agentpb.TunnelFrame
	ctx context.Context
}

func (f *fakeServerTunnel) Send(*agentpb.TunnelFrame) error { return nil }
func (f *fakeServerTunnel) Recv() (*agentpb.TunnelFrame, error) {
	fr, ok := <-f.in
	if !ok {
		return nil, io.EOF
	}
	return fr, nil
}

func TestTunnelLimitsFromEnv(t *testing.T) {
	env := map[string]string{EnvTunnelMaxPerNode: "7", EnvTunnelOpenTimeout: "3s"}
	l, err := TunnelLimitsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil || l.MaxPerNode != 7 || l.OpenTimeout != 3*time.Second || l.MaxPerService != 128 {
		t.Fatalf("limits = %+v err = %v", l, err)
	}
	env[EnvTunnelIdleTimeout] = "soon"
	if _, err := TunnelLimitsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok }); err == nil {
		t.Fatal("bad duration accepted")
	}
}
