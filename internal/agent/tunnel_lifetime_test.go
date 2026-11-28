package agent

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	"github.com/glincker/levelrail/internal/agent/agentpb"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/spec"
)

// The real transport talks to the real agent handler over bufconn: a tunnel
// must outlive the open handshake timeout and carry data the whole time.
func TestTunnelOutlivesOpenTimeoutWithRealAgent(t *testing.T) {
	rt := &tunnelRuntime{}
	var agentCtx context.Context
	r := newTunnelRig(t, []string{CapIngressTunnel}, func(req *agentpb.TunnelOpenRequest) (string, bool) {
		resp, err := executeTunnelOpen(agentCtx, rt, req)
		if err != nil {
			return err.Error(), false
		}
		return resp.GetFailureReason(), false
	})
	r.realAgent = true
	const openTimeout = 200 * time.Millisecond
	r.hub.setLimits(TunnelLimits{OpenTimeout: openTimeout, IdleTimeout: time.Minute})
	agentCtx = withTunnels(context.Background(), agentpbOpener{agentpb.NewAgentServiceClient(r.cc)})

	app, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	go func() {
		for {
			c, err := app.Accept()
			if err != nil {
				return
			}
			go func() { defer func() { _ = c.Close() }(); _, _ = io.Copy(c, c) }()
		}
	}()
	rt.list = []docker.ContainerState{{
		Name: "web-0123abcd", Running: true, Labels: map[string]string{spec.AppLabelKey: "web"},
		Ports: []docker.PortBinding{{ContainerPort: 8080, HostPort: app.Addr().(*net.TCPAddr).Port, HostIP: "127.0.0.1", Protocol: "tcp"}},
	}}

	c, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 8080, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	end := time.Now().Add(3 * openTimeout)
	buf := make([]byte, 5)
	for time.Now().Before(end) {
		if _, err := c.Write([]byte("hello")); err != nil {
			t.Fatalf("tunnel closed early: %v", err)
		}
		if _, err := io.ReadFull(c, buf); err != nil || !bytes.Equal(buf, []byte("hello")) {
			t.Fatalf("tunnel closed early: %v", err)
		}
		time.Sleep(openTimeout / 4)
	}
	select {
	case <-c.Done():
		t.Fatal("tunnel was torn down within its lifetime")
	default:
	}
}

type agentpbOpener struct{ c agentpb.AgentServiceClient }

func (o agentpbOpener) Tunnel(ctx context.Context, opts ...grpc.CallOption) (tunnelClientStream, error) {
	return o.c.Tunnel(ctx, opts...)
}

func TestTunnelLifetimeSentIsRemainingDeadline(t *testing.T) {
	got := make(chan int64, 2)
	r := newTunnelRig(t, []string{CapIngressTunnel}, func(req *agentpb.TunnelOpenRequest) (string, bool) {
		got <- req.GetDeadlineNs()
		return "", false
	})
	r.hub.setLimits(TunnelLimits{OpenTimeout: 200 * time.Millisecond})
	for _, dl := range []time.Time{time.Now().Add(time.Hour), {}} {
		c, err := r.tp.OpenTunnel(context.Background(), testTunnelNode, "web", 80, dl)
		if err != nil {
			t.Fatal(err)
		}
		_ = c.Close()
	}
	if ns := time.Duration(<-got); ns < 59*time.Minute || ns > time.Hour {
		t.Fatalf("lifetime sent = %v, want about 1h", ns)
	}
	if ns := time.Duration(<-got); ns != TunnelMaxDeadline {
		t.Fatalf("zero deadline sent %v, want the lifetime cap", ns)
	}
}

func TestTunnelAgentClampsLifetime(t *testing.T) {
	e := newTunnelEnv(t, echoServer(t))
	start := time.Now()
	_, s := e.open(t, "web", 8080, 1000*TunnelMaxDeadline)
	if s == nil {
		t.Fatal("open refused")
	}
	if clampTunnelLife(1000*TunnelMaxDeadline) != TunnelMaxDeadline || clampTunnelLife(0) != TunnelDefaultDeadline {
		t.Fatal("lifetime not clamped")
	}
	if time.Since(start) > time.Minute {
		t.Fatal("open took too long")
	}
}

func TestTunnelHelloWaitersAreCapped(t *testing.T) {
	r := newTunnelRig(t, []string{CapIngressTunnel}, echoOK)
	r.hub.setLimits(TunnelLimits{OpenTimeout: 5 * time.Second, MaxPendingHellos: 2})
	cl := agentpb.NewAgentServiceClient(r.cc)
	var held []grpc.BidiStreamingClient[agentpb.TunnelFrame, agentpb.TunnelFrame]
	for range 2 {
		st, err := cl.Tunnel(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, st)
	}
	time.Sleep(100 * time.Millisecond)
	st, err := cl.Tunnel(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if _, err := st.Recv(); err == nil || time.Since(start) > time.Second {
		t.Fatalf("third waiter was not refused quickly: %v", err)
	}
	_ = held
}
