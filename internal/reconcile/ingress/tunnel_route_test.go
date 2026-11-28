package ingress

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/agent"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/experimental"
	"github.com/glincker/levelrail/internal/ingress"
	"github.com/glincker/levelrail/internal/meshpath"
	"github.com/glincker/levelrail/internal/reconcile/application"
	"github.com/glincker/levelrail/internal/store"
)

// echoDialer answers every tunnel with an in-memory echo so the gateway path is real.
type echoDialer struct{}

type pipeEnd struct{ net.Conn }

func (p pipeEnd) CloseWrite() error {
	if cw, ok := p.Conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return p.Close()
}

func (echoDialer) OpenTunnel(context.Context, string, string, int) (ingress.TunnelStream, error) {
	a, b := net.Pipe()
	go func() { _, _ = io.Copy(b, b); _ = b.Close() }()
	return pipeEnd{a}, nil
}

func newTestTunnels(t *testing.T, capable bool) *AgentTunnels {
	t.Helper()
	root, err := os.MkdirTemp("", "lrt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	gw, err := ingress.NewTunnelGateway(filepath.Join(root, "s"), echoDialer{}, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = gw.Close() })
	return &AgentTunnels{Gateway: gw, Capable: func(string) bool { return capable }}
}

func TestController_AgentTunnel_DecisionTable(t *testing.T) {
	remote := remoteSvc()
	name := application.ContainerName(remote.Name, remote.Image, "")
	local := remote
	local.NodeID = "node-1"
	tests := []struct {
		name       string
		svc        store.DesiredService
		path       meshpath.Path
		flag       bool
		capable    bool
		wantUnix   bool
		wantReason string
	}{
		{name: "mesh usable keeps the mesh", svc: remote, path: meshpath.Path{Usable: true, Address: "10.181.0.2"}, flag: true, capable: true},
		{name: "no mesh, flag and capability", svc: remote, path: meshpath.Path{Reason: "node has not joined the mesh yet"}, flag: true, capable: true, wantUnix: true, wantReason: "AgentTunnel"},
		{name: "no mesh, flag off", svc: remote, path: meshpath.Path{Reason: "node has not joined the mesh yet"}, capable: true, wantReason: "NoMeshIngressPath"},
		{name: "no mesh, no capability", svc: remote, path: meshpath.Path{Reason: "node has not joined the mesh yet"}, flag: true, wantReason: "NoMeshIngressPath"},
		{name: "local node", svc: local, path: meshpath.Path{Reason: "n/a"}, flag: true, capable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.flag {
				experimental.Set(experimental.AgentIngressTunnel)
			}
			t.Cleanup(experimental.Reset)
			rt := newFakeRuntime()
			rt.containers[name] = &docker.ContainerState{Name: name, Running: true, Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 32768, HostIP: "10.181.0.2"}}}
			applier := &fakeApplier{}
			opts := []Option{WithLogger(slog.New(slog.DiscardHandler)), WithLocalNodeID("node-1"),
				WithNodeUpstreams(fakeNodes{rt: rt, host: "10.181.0.2"}), WithMeshPaths(&fakePaths{path: tt.path})}
			if tt.flag {
				opts = append(opts, WithAgentTunnels(newTestTunnels(t, tt.capable)))
			}
			c := New(&fakeStore{services: []store.DesiredService{tt.svc}}, rt, applier, opts...)
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			cond := crossNodeCond(res)
			if tt.wantReason == "" {
				if cond != nil {
					t.Fatalf("condition = %+v, want none", cond)
				}
				return
			}
			if cond == nil || cond.Reason != tt.wantReason {
				t.Fatalf("condition = %+v, want reason %s", cond, tt.wantReason)
			}
			if tt.wantUnix {
				dial := proxyHandler(t, applier).Upstreams[0].Dial
				if !strings.HasPrefix(dial, "unix//") {
					t.Fatalf("dial = %q, want a unix upstream", dial)
				}
			} else if len(applier.routes(t)) != 0 {
				t.Fatal("want no route")
			}
		})
	}
}

func TestController_AgentTunnel_EndToEndAndCleanup(t *testing.T) {
	experimental.Set(experimental.AgentIngressTunnel)
	t.Cleanup(experimental.Reset)
	svc := remoteSvc()
	name := application.ContainerName(svc.Name, svc.Image, "")
	rt := newFakeRuntime()
	rt.containers[name] = &docker.ContainerState{Name: name, Running: true, Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: 32768, HostIP: "127.0.0.1"}}}
	st := &fakeStore{services: []store.DesiredService{svc}}
	applier := &fakeApplier{}
	tunnels := newTestTunnels(t, true)
	c := New(st, rt, applier, WithLogger(slog.New(slog.DiscardHandler)), WithLocalNodeID("node-1"),
		WithNodeUpstreams(fakeNodes{rt: rt, host: "10.181.0.2"}),
		WithMeshPaths(&fakePaths{path: meshpath.Path{Reason: "node has not joined the mesh yet"}}), WithAgentTunnels(tunnels))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	dial := proxyHandler(t, applier).Upstreams[0].Dial
	sock := strings.TrimPrefix(dial, "unix/")
	conn, err := net.DialTimeout("unix", sock, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil || string(buf) != "ping" {
		t.Fatalf("echo = %q, %v", buf, err)
	}
	_ = conn.Close()

	st.services = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Fatalf("socket should be removed once the target is gone, stat err = %v", err)
	}
}

type errOpener struct{ err error }

func (e errOpener) OpenTunnel(context.Context, string, string, uint16, time.Time) (*agent.TunnelConn, error) {
	return nil, e.err
}

func TestAgentTunnelDialer_Errors(t *testing.T) {
	tests := []struct {
		name    string
		port    int
		err     error
		wantIs  error
		wantNot error
	}{
		{name: "unsupported", port: 80, err: agent.ErrTunnelUnsupported, wantIs: ingress.ErrTunnelUnsupported},
		{name: "lacks capability", port: 80, err: agent.ErrAgentLacksCapability, wantIs: ingress.ErrTunnelUnsupported},
		{name: "offline", port: 80, err: agent.ErrNodeOffline, wantIs: ingress.ErrTunnelOffline},
		{name: "refused stays a 502", port: 80, err: agent.ErrTunnelRefused, wantIs: agent.ErrTunnelRefused, wantNot: ingress.ErrTunnelOffline},
		{name: "port too large", port: 70000},
		{name: "port zero", port: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AgentTunnelDialer{Opener: errOpener{err: tt.err}}.OpenTunnel(context.Background(), "n", "web", tt.port)
			if err == nil {
				t.Fatal("want an error")
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Fatalf("err = %v, want %v", err, tt.wantIs)
			}
			if tt.wantNot != nil && errors.Is(err, tt.wantNot) {
				t.Fatalf("err = %v, must not match %v", err, tt.wantNot)
			}
		})
	}
}

func TestTranslateTunnelTimeout(t *testing.T) {
	if err := translateTunnelError(agent.ErrTunnelTimeout); !errors.Is(err, ingress.ErrTunnelTimeout) {
		t.Fatalf("got %v", err)
	}
}
