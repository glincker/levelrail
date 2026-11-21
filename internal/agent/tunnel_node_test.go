package agent

import (
	"bytes"
	"context"
	"io"
	"net"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// fakeTunnelStream is the in-memory Tunnel RPC: tiny channels give real back-pressure.
type fakeTunnelStream struct {
	ctx      context.Context
	toServer chan *agentpb.TunnelFrame
	toAgent  chan *agentpb.TunnelFrame
}

func (f *fakeTunnelStream) Send(m *agentpb.TunnelFrame) error {
	select {
	case f.toServer <- m:
		return nil
	case <-f.ctx.Done():
		return f.ctx.Err()
	}
}

func (f *fakeTunnelStream) Recv() (*agentpb.TunnelFrame, error) {
	select {
	case m := <-f.toAgent:
		return m, nil
	case <-f.ctx.Done():
		return nil, f.ctx.Err()
	}
}
func (f *fakeTunnelStream) Header() (metadata.MD, error) { return nil, nil }
func (f *fakeTunnelStream) Trailer() metadata.MD         { return nil }
func (f *fakeTunnelStream) CloseSend() error             { return nil }
func (f *fakeTunnelStream) Context() context.Context     { return f.ctx }
func (f *fakeTunnelStream) SendMsg(any) error            { return nil }
func (f *fakeTunnelStream) RecvMsg(any) error            { return nil }

type fakeOpener struct{ ch chan *fakeTunnelStream }

func (o *fakeOpener) Tunnel(ctx context.Context, _ ...grpc.CallOption) (tunnelClientStream, error) {
	s := &fakeTunnelStream{ctx: ctx, toServer: make(chan *agentpb.TunnelFrame, 1), toAgent: make(chan *agentpb.TunnelFrame, 1)}
	o.ch <- s
	return s, nil
}

type tunnelRuntime struct {
	fakeRuntime
	list []docker.ContainerState
}

func (r *tunnelRuntime) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return r.list, nil
}

type tunnelEnv struct {
	ctx    context.Context
	m      *tunnelManager
	opener *fakeOpener
	rt     *tunnelRuntime
	port   int
	cancel context.CancelFunc
}

func newTunnelEnv(t *testing.T, appPort int) *tunnelEnv {
	t.Helper()
	experimental.Set(experimental.AgentIngressTunnel)
	t.Cleanup(experimental.Reset)
	base, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	op := &fakeOpener{ch: make(chan *fakeTunnelStream, 8)}
	ctx := withTunnels(base, op)
	rt := &tunnelRuntime{list: []docker.ContainerState{{
		Name: "web-0123abcd", Running: true, Labels: map[string]string{spec.AppLabelKey: "web"},
		Ports: []docker.PortBinding{{ContainerPort: 8080, HostPort: appPort, HostIP: "127.0.0.1", Protocol: "tcp"}},
	}}}
	return &tunnelEnv{ctx: ctx, m: tunnelsFrom(ctx), opener: op, rt: rt, port: appPort, cancel: cancel}
}

func (e *tunnelEnv) open(t *testing.T, service string, containerPort uint32, life time.Duration) (*agentpb.TunnelOpenResponse, *fakeTunnelStream) {
	t.Helper()
	id := bytes.Repeat([]byte{9}, TunnelIDLen)
	resp, err := executeTunnelOpen(e.ctx, e.rt, &agentpb.TunnelOpenRequest{TunnelId: id, Service: service, ContainerPort: containerPort, DeadlineNs: int64(life)})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetFailureReason() != "" {
		return resp, nil
	}
	select {
	case s := <-e.opener.ch:
		hello := recvFrame(t, s)
		if !bytes.Equal(hello.GetHello().GetTunnelId(), id) {
			t.Fatalf("hello id mismatch: %v", hello)
		}
		return resp, s
	case <-time.After(3 * time.Second):
		t.Fatal("tunnel stream never opened")
		return nil, nil
	}
}

func recvFrame(t *testing.T, s *fakeTunnelStream) *agentpb.TunnelFrame {
	t.Helper()
	select {
	case f := <-s.toServer:
		return f
	case <-time.After(3 * time.Second):
		t.Fatal("no frame from agent")
		return nil
	}
}

func (f *fakeTunnelStream) push(m *agentpb.TunnelFrame) { f.toAgent <- m }

func dataFrame(b []byte) *agentpb.TunnelFrame {
	return &agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Data{Data: b}}
}

func halfClose() *agentpb.TunnelFrame {
	return &agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_HalfClose{HalfClose: &agentpb.Empty{}}}
}

// echoServer echoes until the peer half-closes, then closes its write side.
func echoServer(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				_, _ = io.Copy(c, c)
				if tc, ok := c.(*net.TCPConn); ok {
					_ = tc.CloseWrite()
				}
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitDone(t *testing.T, m *tunnelManager) {
	t.Helper()
	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tunnel did not end")
	}
}

func TestTunnelRoundTripAndHalfClose(t *testing.T) {
	e := newTunnelEnv(t, echoServer(t))
	_, s := e.open(t, "web", 8080, time.Minute)
	s.push(dataFrame([]byte("ping")))
	var got []byte
	for len(got) < 4 {
		got = append(got, recvFrame(t, s).GetData()...)
	}
	if string(got) != "ping" {
		t.Fatalf("echo = %q", got)
	}
	s.push(halfClose())
	if recvFrame(t, s).GetHalfClose() == nil {
		t.Fatal("expected half_close after the app closed its side")
	}
	waitDone(t, e.m)
}

func TestTunnelLargeTransferWithBackPressure(t *testing.T) {
	e := newTunnelEnv(t, echoServer(t))
	_, s := e.open(t, "web", 8080, time.Minute)
	const total = 8 << 20
	chunk := bytes.Repeat([]byte{'x'}, TunnelMaxFrameData)
	go func() {
		for sent := 0; sent < total; sent += len(chunk) {
			s.push(dataFrame(chunk))
		}
		s.push(halfClose())
	}()
	got := 0
	for {
		f := recvFrame(t, s)
		if f.GetHalfClose() != nil {
			break
		}
		if len(f.GetData()) > TunnelMaxFrameData {
			t.Fatal("oversized frame from agent")
		}
		got += len(f.GetData())
	}
	if got != total {
		t.Fatalf("got %d bytes, want %d", got, total)
	}
	waitDone(t, e.m)
}

func TestTunnelBoundedWhenPeerStopsReading(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	var written atomic.Int64
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer func() { _ = c.Close() }()
		buf := make([]byte, 64*1024)
		for i := 0; i < 4096; i++ {
			n, err := c.Write(buf)
			written.Add(int64(n))
			if err != nil {
				return
			}
		}
	}()
	e := newTunnelEnv(t, ln.Addr().(*net.TCPAddr).Port)
	_, _ = e.open(t, "web", 8080, time.Minute)
	time.Sleep(400 * time.Millisecond)
	if w := written.Load(); w > 64<<20 {
		t.Fatalf("app wrote %d bytes into a stalled tunnel; the pump is not applying back-pressure", w)
	}
}

func TestTunnelDeadlineAndIdleExpire(t *testing.T) {
	e := newTunnelEnv(t, echoServer(t))
	_, _ = e.open(t, "web", 8080, 150*time.Millisecond)
	waitDone(t, e.m)

	e2 := newTunnelEnv(t, echoServer(t))
	e2.m.idle = 150 * time.Millisecond
	_, s := e2.open(t, "web", 8080, time.Minute)
	if f := recvFrame(t, s); f.GetReset_().GetReason() != "idle timeout" {
		t.Fatalf("expected idle reset, got %v", f)
	}
	waitDone(t, e2.m)
}

func TestTunnelDeadlineIsClamped(t *testing.T) {
	if TunnelMaxDeadline <= 0 || TunnelDefaultDeadline > TunnelMaxDeadline {
		t.Fatal("deadline constants are inconsistent")
	}
}

func TestTunnelRefusals(t *testing.T) {
	e := newTunnelEnv(t, echoServer(t))
	for _, host := range []string{"", "0.0.0.0", "10.1.2.3", "::", "localhost"} {
		e.rt.list[0].Ports[0].HostIP = host
		if r, _ := e.open(t, "web", 8080, time.Minute); r.GetFailureReason() == "" {
			t.Fatalf("host %q was accepted", host)
		}
	}
	e.rt.list[0].Ports[0].HostIP = "127.0.0.1"

	cases := map[string]struct {
		service string
		port    uint32
	}{
		"unknown service":   {"api", 8080},
		"unpublished port":  {"web", 9090},
		"port zero":         {"web", 0},
		"port out of range": {"web", 70000},
		"empty service":     {"", 8080},
		"bad service chars": {"web/../x", 8080},
	}
	for name, c := range cases {
		if r, _ := e.open(t, c.service, c.port, time.Minute); r.GetFailureReason() == "" {
			t.Fatalf("%s was accepted", name)
		}
	}
	// A longer service sharing the prefix must not be served as "web".
	e.rt.list[0].Name = "web-2-0123abcd"
	if r, _ := e.open(t, "web", 8080, time.Minute); r.GetFailureReason() == "" {
		t.Fatal("container of a different service was accepted")
	}
	e.rt.list[0].Name, e.rt.list[0].Running = "web-0123abcd", false
	if r, _ := e.open(t, "web", 8080, time.Minute); r.GetFailureReason() == "" {
		t.Fatal("stopped container was accepted")
	}
	e.rt.list[0].Running = true
	resp, err := executeTunnelOpen(e.ctx, e.rt, &agentpb.TunnelOpenRequest{TunnelId: []byte{1}, Service: "web", ContainerPort: 8080})
	if err != nil || resp.GetFailureReason() == "" {
		t.Fatalf("short tunnel id accepted: %v %v", resp, err)
	}
}

func TestTunnelRefusedWhenAppNotListening(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	e := newTunnelEnv(t, port)
	if r, _ := e.open(t, "web", 8080, time.Minute); r.GetFailureReason() == "" {
		t.Fatal("dial to a closed port reported success")
	}
	if e.m.total != 0 {
		t.Fatal("slot leaked after a failed dial")
	}
}

func TestTunnelFlagOff(t *testing.T) {
	experimental.Set()
	defer experimental.Reset()
	if _, err := executeTunnelOpen(context.Background(), nil, &agentpb.TunnelOpenRequest{}); err == nil {
		t.Fatal("flag off must stay unsupported")
	}
}

func TestTunnelConcurrencyLimit(t *testing.T) {
	e := newTunnelEnv(t, echoServer(t))
	e.m.maxService = 1
	_, s := e.open(t, "web", 8080, time.Minute)
	if r, _ := e.open(t, "web", 8080, time.Minute); !strings.Contains(r.GetFailureReason(), "limit") {
		t.Fatalf("second tunnel not limited: %v", r)
	}
	s.push(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Reset_{Reset_: &agentpb.TunnelReset{Reason: "done"}}})
	waitDone(t, e.m)
	e.m.maxService, e.m.maxNode = 5, 1
	_, s = e.open(t, "web", 8080, time.Minute)
	if r, _ := e.open(t, "web", 8080, time.Minute); r.GetFailureReason() == "" {
		t.Fatal("node limit not enforced")
	}
	s.push(halfClose())
	s.push(halfClose())
}

func TestTunnelResetAndErrorsCloseTheApp(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	closed := make(chan struct{})
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, c)
		close(closed)
	}()
	e := newTunnelEnv(t, ln.Addr().(*net.TCPAddr).Port)
	_, s := e.open(t, "web", 8080, time.Minute)
	s.push(dataFrame(make([]byte, TunnelMaxFrameData+1)))
	if r := recvFrame(t, s).GetReset_().GetReason(); r != "frame too large" || len(r) > TunnelMaxResetReason {
		t.Fatalf("reset reason = %q", r)
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("app connection stayed open after a tunnel error")
	}
	waitDone(t, e.m)
}

func TestTunnelNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	port := echoServer(t)
	for i := 0; i < 20; i++ {
		e := newTunnelEnv(t, port)
		_, s := e.open(t, "web", 8080, time.Minute)
		s.push(dataFrame([]byte("x")))
		recvFrame(t, s)
		// Cancel the session, as a dropped connection would.
		e.cancel()
		waitDone(t, e.m)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= before+4 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("goroutines grew from %d to %d", before, runtime.NumGoroutine())
}
