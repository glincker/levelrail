package agent

import (
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"google.golang.org/grpc"

	"github.com/glincker/levelrail/internal/agent/agentpb"
	"github.com/glincker/levelrail/internal/docker"
	"github.com/glincker/levelrail/internal/experimental"
	"github.com/glincker/levelrail/internal/spec"
)

// Agent-side tunnel limits. The node enforces them on its own so a control
// plane bug cannot exhaust the host; the cap can be seeded from the env.
const (
	// TunnelMaxStreamsPerNode caps concurrent tunnels on this node.
	TunnelMaxStreamsPerNode = 256
	// TunnelMaxStreamsPerService caps concurrent tunnels to one service.
	TunnelMaxStreamsPerService = 128
	// TunnelDialTimeout bounds the loopback dial to the app.
	TunnelDialTimeout = 3 * time.Second
	// TunnelIdleTimeout ends a tunnel with no bytes in either direction.
	TunnelIdleTimeout = 10 * time.Minute
	// TunnelDefaultDeadline is the lifetime used when the request names none.
	TunnelDefaultDeadline = time.Hour
	// TunnelMaxDeadline clamps a requested lifetime.
	TunnelMaxDeadline = 24 * time.Hour
	// tunnelResetGrace is how long a reset frame may take to leave before teardown.
	tunnelResetGrace = time.Second
	// tunnelMaxStreamsEnv seeds the per-node cap, per the runtime settings rule.
	tunnelMaxStreamsEnv = "APP_AGENT_TUNNEL_MAX_STREAMS"
	// tunnelHashLen mirrors the application container name hash length.
	tunnelHashLen = 8
)

var errTunnelNotLoopback = errors.New("agent: tunnel only dials a loopback address")

// tunnelClientStream is the client side of the Tunnel RPC.
type tunnelClientStream = grpc.BidiStreamingClient[agentpb.TunnelFrame, agentpb.TunnelFrame]

// tunnelOpener opens the Tunnel RPC on the session's connection.
type tunnelOpener interface {
	Tunnel(ctx context.Context, opts ...grpc.CallOption) (tunnelClientStream, error)
}

// tunnelManager tracks this node's tunnels and their caps.
type tunnelManager struct {
	base        context.Context
	opener      tunnelOpener
	maxNode     int
	maxService  int
	idle        time.Duration
	dialTimeout time.Duration

	mu         sync.Mutex
	total      int
	perService map[string]int
	rr         map[string]uint32
	wg         sync.WaitGroup
}

type tunnelCtxKey struct{}

// withTunnels binds a manager to the session context; tunnels end with it.
func withTunnels(ctx context.Context, opener tunnelOpener) context.Context {
	m := &tunnelManager{
		base: ctx, opener: opener, maxNode: TunnelMaxStreamsPerNode, maxService: TunnelMaxStreamsPerService,
		idle: TunnelIdleTimeout, dialTimeout: TunnelDialTimeout, perService: map[string]int{}, rr: map[string]uint32{},
	}
	if n, err := strconv.Atoi(os.Getenv(tunnelMaxStreamsEnv)); err == nil && n > 0 {
		m.maxNode = n
	}
	return context.WithValue(ctx, tunnelCtxKey{}, m)
}

func tunnelsFrom(ctx context.Context) *tunnelManager {
	m, _ := ctx.Value(tunnelCtxKey{}).(*tunnelManager)
	return m
}

func (m *tunnelManager) acquire(service string) (release func(), ok bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.total >= m.maxNode || m.perService[service] >= m.maxService {
		return nil, false
	}
	m.total++
	m.perService[service]++
	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			defer m.mu.Unlock()
			m.total--
			if m.perService[service]--; m.perService[service] <= 0 {
				delete(m.perService, service)
			}
		})
	}, true
}

// tunnelLoopbackControl refuses any resolved remote that is not a loopback IP.
func tunnelLoopbackControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return errTunnelNotLoopback
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return errTunnelNotLoopback
	}
	return nil
}

func refuse(reason string) *agentpb.TunnelOpenResponse {
	return &agentpb.TunnelOpenResponse{FailureReason: reason}
}

// executeTunnelOpen validates the request, resolves the service's published
// loopback port from this node's own containers, dials it, and then serves
// the tunnel on its own Tunnel RPC. The wire never supplies an address.
func executeTunnelOpen(ctx context.Context, rt docker.Runtime, req *agentpb.TunnelOpenRequest) (*agentpb.TunnelOpenResponse, error) {
	if !experimental.Enabled(experimental.AgentIngressTunnel) {
		return nil, ErrTunnelUnsupported
	}
	m := tunnelsFrom(ctx)
	if m == nil {
		return refuse("tunnels unavailable on this session"), nil
	}
	if len(req.GetTunnelId()) != TunnelIDLen {
		return refuse("invalid tunnel id"), nil
	}
	port := req.GetContainerPort()
	if port == 0 || port > 65535 || !validTunnelService(req.GetService()) {
		return refuse("invalid service or port"), nil
	}
	service := req.GetService()
	hostPort, reason := resolveTunnelPort(ctx, m, rt, service, int(port))
	if reason != "" {
		return refuse(reason), nil
	}
	release, ok := m.acquire(service)
	if !ok {
		return refuse("tunnel limit reached"), nil
	}
	dialCtx, cancel := context.WithTimeout(ctx, m.dialTimeout)
	defer cancel()
	d := &net.Dialer{Control: tunnelLoopbackControl}
	conn, err := d.DialContext(dialCtx, "tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(hostPort)))
	if err != nil {
		release()
		return refuse("service port not reachable"), nil
	}
	life := clampTunnelLife(time.Duration(req.GetDeadlineNs()))
	id := append([]byte(nil), req.GetTunnelId()...)
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer release()
		m.serve(id, service, conn, life)
	}()
	return &agentpb.TunnelOpenResponse{}, nil
}

// clampTunnelLife applies the default to a missing lifetime and the node's own cap to a long one.
func clampTunnelLife(life time.Duration) time.Duration {
	if life <= 0 {
		return TunnelDefaultDeadline
	}
	return min(life, TunnelMaxDeadline)
}

// validTunnelService accepts only a plain service name, so it cannot widen the prefix match.
func validTunnelService(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	for _, r := range s {
		ok := r == '-' || r == '_' || r == '.' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// ownsServiceContainer matches <service>-<hash> and <service>-<hash>-r<N>, so
// a longer service name sharing the prefix is never mistaken for this one.
func ownsServiceContainer(service, name string) bool {
	suffix, ok := strings.CutPrefix(name, service+"-")
	if !ok || len(suffix) < tunnelHashLen {
		return false
	}
	for _, r := range suffix[:tunnelHashLen] {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	rest := suffix[tunnelHashLen:]
	if rest == "" {
		return true
	}
	n, ok := strings.CutPrefix(rest, "-r")
	if !ok || n == "" {
		return false
	}
	for _, r := range n {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// resolveTunnelPort picks one of the service's active containers (see
// pickTunnelContainer) and returns its published loopback host port.
func resolveTunnelPort(ctx context.Context, m *tunnelManager, rt docker.Runtime, service string, containerPort int) (int, string) {
	containers, err := rt.ListByPrefix(ctx, service+"-")
	if err != nil {
		return 0, "service lookup failed"
	}
	cands := activeTunnelCandidates(containers, service, containerPort)
	if len(cands) == 0 {
		return 0, "no published loopback port for service"
	}
	return cands[m.nextReplica(service)%len(cands)].port, ""
}

type tunnelCandidate struct {
	name string
	port int
}

// activeTunnelCandidates keeps only running, Levelrail managed containers of
// this service that are not failing a healthcheck and are not the older
// release of a healthy successor (a blue-green overlap), sorted by name.
func activeTunnelCandidates(containers []docker.ContainerState, service string, containerPort int) []tunnelCandidate {
	type entry struct {
		c    *docker.ContainerState
		port int
	}
	var ready []entry
	superseded := map[string]bool{}
	for i := range containers {
		c := &containers[i]
		if !c.Running || c.LabelsUnknown || c.Labels[spec.AppLabelKey] == "" || !ownsServiceContainer(service, c.Name) {
			continue
		}
		if c.Health == "unhealthy" || c.Health == "starting" {
			continue
		}
		hp := loopbackHostPort(c.Ports, containerPort)
		if hp == 0 {
			continue
		}
		ready = append(ready, entry{c, hp})
		for _, rel := range strings.Split(c.Labels[spec.SupersedesLabelKey], ",") {
			if rel != "" {
				superseded[rel] = true
			}
		}
	}
	var out []tunnelCandidate
	for _, e := range ready {
		if superseded[tunnelRelease(service, e.c.Name)] {
			continue
		}
		out = append(out, tunnelCandidate{e.c.Name, e.port})
	}
	slices.SortFunc(out, func(a, b tunnelCandidate) int { return strings.Compare(a.name, b.name) })
	return out
}

// tunnelRelease is the <service>-<hash> part of a container name, shared by its replicas.
func tunnelRelease(service, name string) string {
	return name[:len(service)+1+tunnelHashLen]
}

// nextReplica rotates replicas per service, one step per tunnel open.
func (m *tunnelManager) nextReplica(service string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := m.rr[service]
	m.rr[service] = n + 1
	return int(n)
}

func loopbackHostPort(ports []docker.PortBinding, containerPort int) int {
	for _, p := range ports {
		if p.ContainerPort != containerPort || (p.Protocol != "" && p.Protocol != "tcp") {
			continue
		}
		if ip := net.ParseIP(p.HostIP); ip == nil || !ip.IsLoopback() {
			continue
		}
		if p.HostPort > 0 && p.HostPort <= 65535 {
			return p.HostPort
		}
	}
	return 0
}

// tunnelRun is the state of one running tunnel.
type tunnelRun struct {
	stream  tunnelClientStream
	conn    net.Conn
	cancel  context.CancelFunc
	sendMu  sync.Mutex
	idle    *time.Timer
	idleFor time.Duration
	failed  atomic.Pointer[string]
	bytes   atomic.Int64
}

func (r *tunnelRun) touch() { r.idle.Reset(r.idleFor) }

// fail records the first error and tears the tunnel down after a short reset.
func (r *tunnelRun) fail(reason string) {
	if !r.failed.CompareAndSwap(nil, &reason) {
		return
	}
	r.reset(reason)
	r.cancel()
}

// reset sends a bounded reason; a blocked send is abandoned after a grace period.
func (r *tunnelRun) reset(reason string) {
	if len(reason) > TunnelMaxResetReason {
		reason = reason[:TunnelMaxResetReason]
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.sendMu.Lock()
		defer r.sendMu.Unlock()
		_ = r.stream.Send(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Reset_{Reset_: &agentpb.TunnelReset{Reason: reason}}})
	}()
	select {
	case <-done:
	case <-time.After(tunnelResetGrace):
	}
}

func (r *tunnelRun) send(f *agentpb.TunnelFrame) error {
	r.sendMu.Lock()
	defer r.sendMu.Unlock()
	return r.stream.Send(f)
}

// serve opens the Tunnel RPC, binds it with the hello, and pumps both ways
// until both sides close, a reset, the idle timeout or the absolute deadline.
func (m *tunnelManager) serve(id []byte, service string, conn net.Conn, life time.Duration) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(m.base, life)
	defer cancel()
	defer func() { _ = conn.Close() }()

	stream, err := m.opener.Tunnel(ctx)
	if err != nil {
		slog.DebugContext(ctx, "tunnel stream open failed", slog.String("service", service))
		return
	}
	r := &tunnelRun{stream: stream, conn: conn, cancel: cancel, idleFor: m.idle}
	r.idle = time.AfterFunc(m.idle, func() { r.fail("idle timeout") })
	defer r.idle.Stop()

	if err := r.send(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Hello{Hello: &agentpb.TunnelHello{TunnelId: id}}}); err != nil {
		return
	}
	var pumps sync.WaitGroup
	pumps.Add(2)
	go func() { defer pumps.Done(); r.toStream(ctx) }()
	go func() { defer pumps.Done(); r.toConn(ctx) }()
	finished := make(chan struct{})
	go func() { pumps.Wait(); close(finished) }()
	select {
	case <-finished:
	case <-ctx.Done():
	}
	// Closing the conn and cancelling the stream unblock both pumps, so none outlives the tunnel.
	cancel()
	_ = conn.Close()
	<-finished

	outcome := "ok"
	if p := r.failed.Load(); p != nil {
		outcome = *p
	} else if ctx.Err() != nil && errors.Is(context.Cause(ctx), context.DeadlineExceeded) {
		outcome = "deadline"
	}
	slog.DebugContext(m.base, "tunnel closed",
		slog.String("tunnel", hex.EncodeToString(id)), slog.String("service", service), slog.String("outcome", outcome),
		slog.Int64("bytes", r.bytes.Load()), slog.Duration("elapsed", time.Since(start)))
}

// toStream copies app bytes into data frames; Send blocks on HTTP/2 flow control, which is the back-pressure.
func (r *tunnelRun) toStream(ctx context.Context) {
	buf := make([]byte, TunnelMaxFrameData)
	for {
		n, err := r.conn.Read(buf)
		if n > 0 {
			r.touch()
			r.bytes.Add(int64(n))
			if serr := r.send(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_Data{Data: buf[:n]}}); serr != nil {
				return
			}
		}
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, io.EOF) {
				_ = r.send(&agentpb.TunnelFrame{Frame: &agentpb.TunnelFrame_HalfClose{HalfClose: &agentpb.Empty{}}})
				return
			}
			r.fail("app read error")
			return
		}
	}
}

// toConn writes peer frames to the app; a slow app stalls Recv, which stalls the peer.
func (r *tunnelRun) toConn(ctx context.Context) {
	for {
		f, err := r.stream.Recv()
		if err != nil {
			if ctx.Err() == nil {
				r.cancel()
			}
			return
		}
		switch fr := f.GetFrame().(type) {
		case *agentpb.TunnelFrame_Data:
			if len(fr.Data) > TunnelMaxFrameData {
				r.fail("frame too large")
				return
			}
			r.touch()
			r.bytes.Add(int64(len(fr.Data)))
			if _, werr := r.conn.Write(fr.Data); werr != nil {
				if ctx.Err() == nil {
					r.fail("app write error")
				}
				return
			}
		case *agentpb.TunnelFrame_HalfClose:
			if tc, ok := r.conn.(*net.TCPConn); ok {
				_ = tc.CloseWrite()
			}
			return
		case *agentpb.TunnelFrame_Reset_:
			r.cancel()
			return
		default:
			r.fail("protocol error")
			return
		}
	}
}
