package agent

// This file: the agent-side connection logic: DialEnroll
// (once, ADR 003's join-token exchange) and RunSession (thereafter, the
// one persistent connection), executing incoming requests against a
// real docker.Runtime via Execute (execute.go) and relaying its own
// Docker events back up. What actually runs this against a real
// scheduled process is cmd/levelrail-agent's thin main(), not built in
// this pass; this package is the tested library it will call into.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
)

// defaultHeartbeatInterval is how often RunSession sends an unprompted
// Heartbeat frame up the Session stream while it stays open. This is
// the one thing internal/agent.Server's own last_seen_at freshness
// actually depends on now (server.go's mux onHeartbeat callback): the
// stream staying open is no longer enough on its own, so this interval,
// not a server-side timer, is what keeps a healthy node looking healthy.
// Matches the observability design's own metrics-collection cadence
// (15s), same reasoning nodeHeartbeatInterval's own doc comment already
// gives for reusing that number rather than inventing a fresh one.
const defaultHeartbeatInterval = 15 * time.Second

// defaultKeepaliveTime/defaultKeepaliveTimeout configure this
// connection's HTTP/2-level PING keepalive, the transport-level backstop
// behind the application-level Heartbeat frame above: a frozen agent
// process (SIGSTOP'd, deadlocked, or otherwise not actually running any
// goroutines, as opposed to having exited) cannot service an incoming
// PING either, since responding requires a scheduled goroutine just as
// much as sending a Heartbeat frame does. Without this, the underlying
// TCP/TLS connection for a frozen-but-not-crashed agent can sit open
// indefinitely with no Heartbeat frame ever arriving again, but also no
// Recv() error ever firing to end Session on the control plane's own
// side either. 10s/10s bounds worst-case detection at roughly
// defaultKeepaliveTime+defaultKeepaliveTimeout (20s), comfortably under
// internal/reconcile/nodehealth's own 45s staleness timeout
// (APP_NODE_HEARTBEAT_TIMEOUT) so the transport itself, not just the
// next reconcile pass, notices and tears the connection down.
const (
	defaultKeepaliveTime    = 10 * time.Second
	defaultKeepaliveTimeout = 10 * time.Second
)

// Identity is what an enrolled node needs to reconnect: its own client
// certificate/key and the control plane's CA certificate, all PEM,
// exactly EnrollResponse's three credential fields. cmd/levelrail-agent
// owns persisting/loading this to/from disk across restarts; this
// package only consumes it.
type Identity struct {
	NodeID        string
	ClientCertPEM []byte
	ClientKeyPEM  []byte
	CACertPEM     []byte
}

// DialEnroll connects to addr and exchanges joinToken for an Identity.
//
// The connection for this one call is not verified against any CA
// (InsecureSkipVerify): there is no CA certificate to verify against
// yet, obtaining one is what this call is for. Trust here rests
// entirely on joinToken's own secrecy, a trust-on-first-use model (the
// same one k3s and Nomad's own join-token bootstrapping use), not on
// TLS server verification: a real, deliberate tradeoff, not an
// oversight. An attacker able to both intercept this one connection and
// obtain a valid, unexpired, not-yet-used join token could complete a
// fraudulent enrollment; the join token being a genuine secret (minted
// server-side, shown once, single-use) is what actually
// carries the security weight here, not this connection's transport.
// Every connection after this one (RunSession below, and any future
// re-enrollment once an Identity already exists) verifies the server
// certificate against the CA this call returns, closing that window to
// this one bootstrap step only.
func DialEnroll(ctx context.Context, addr, joinToken, nodeName string) (*Identity, error) {
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(
		credentials.NewTLS(&tls.Config{InsecureSkipVerify: true, NextProtos: []string{"h2"}}), //nolint:gosec // TOFU bootstrap, see doc comment above
	))
	if err != nil {
		return nil, fmt.Errorf("agent: dial %q for enrollment: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := agentpb.NewAgentServiceClient(conn).Enroll(ctx, &agentpb.EnrollRequest{
		JoinToken: joinToken,
		NodeName:  nodeName,
	})
	if err != nil {
		return nil, fmt.Errorf("agent: enroll: %w", err)
	}

	return &Identity{
		NodeID:        resp.GetNodeId(),
		ClientCertPEM: resp.GetClientCertPem(),
		ClientKeyPEM:  resp.GetClientKeyPem(),
		CACertPEM:     resp.GetCaCertPem(),
	}, nil
}

// agentClientStream is the narrow surface serveSession needs from the
// agent's side of the Session RPC, so tests can fake it without a real
// network connection. agentpb.AgentService_SessionClient satisfies this
// structurally.
type agentClientStream interface {
	Send(*agentpb.AgentMessage) error
	Recv() (*agentpb.ControlMessage, error)
}

// RunSession dials addr with id's mTLS credentials (verified against
// id.CACertPEM, closing DialEnroll's own TOFU window), opens the one
// persistent Session stream ADR 003 describes, and serves every
// incoming AgentRequest against rt until ctx is cancelled or the
// connection fails. Returns the error that ended the session (nil only
// if ctx itself was the cause); never retries or reconnects on its own,
// that's cmd/levelrail-agent's own reconnect loop's job (ADR 003's
// Consequences section's own "real, tested"
// reconnection/backpressure/version-negotiation requirement),
// kept out of this function so it stays a single, directly testable
// connection attempt rather than a policy about how many times or how
// fast to retry.
func RunSession(ctx context.Context, addr string, id *Identity, rt docker.Runtime, logger *slog.Logger, opts ...SessionOption) error {
	if logger == nil {
		logger = slog.Default()
	}

	var cfg sessionConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	cert, err := tls.X509KeyPair(id.ClientCertPEM, id.ClientKeyPEM)
	if err != nil {
		return fmt.Errorf("agent: parse identity certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(id.CACertPEM) {
		return fmt.Errorf("agent: parse CA certificate: no valid certificate found")
	}

	keepaliveTime, keepaliveTimeout := defaultKeepaliveTime, defaultKeepaliveTimeout
	if cfg.keepaliveTime > 0 {
		keepaliveTime = cfg.keepaliveTime
	}
	if cfg.keepaliveTimeout > 0 {
		keepaliveTimeout = cfg.keepaliveTimeout
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		NextProtos:   []string{"h2"},
	})), grpc.WithKeepaliveParams(keepalive.ClientParameters{
		Time:                keepaliveTime,
		Timeout:             keepaliveTimeout,
		PermitWithoutStream: true, // this connection's whole purpose is the one long-lived Session stream, so keep pinging even between requests
	}))
	if err != nil {
		return fmt.Errorf("agent: dial %q: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	stream, err := agentpb.NewAgentServiceClient(conn).Session(ctx)
	if err != nil {
		return fmt.Errorf("agent: open session: %w", err)
	}

	heartbeatInterval := defaultHeartbeatInterval
	if cfg.heartbeatInterval > 0 {
		heartbeatInterval = cfg.heartbeatInterval
	}

	return serveSession(ctx, stream, rt, cfg.builder, heartbeatInterval, logger)
}

// sessionConfig holds RunSession's optional wiring.
type sessionConfig struct {
	builder           BuildRunner
	heartbeatInterval time.Duration
	keepaliveTime     time.Duration
	keepaliveTimeout  time.Duration
}

// SessionOption configures optional RunSession behavior.
type SessionOption func(*sessionConfig)

// WithBuildRunner lets this node accept builds dispatched to it by the
// control plane. Without one, a dispatched build is rejected with a clear
// error instead of failing partway through: an agent whose local BuildKit
// is unreachable still serves every container operation normally.
func WithBuildRunner(runner BuildRunner) SessionOption {
	return func(c *sessionConfig) { c.builder = runner }
}

// WithHeartbeatInterval overrides how often RunSession sends an
// unprompted Heartbeat frame up the Session stream. Without one
// configured, defaultHeartbeatInterval applies. cmd/levelrail-agent's
// own main.go reads APP_NODE_HEARTBEAT_INTERVAL and passes the parsed
// duration here, the project's "no hardcoded thresholds, use env vars"
// rule; this package itself never reads the environment directly.
func WithHeartbeatInterval(d time.Duration) SessionOption {
	return func(c *sessionConfig) { c.heartbeatInterval = d }
}

// WithKeepalive overrides this connection's HTTP/2 PING keepalive
// timing. Without one configured, defaultKeepaliveTime/
// defaultKeepaliveTimeout apply. cmd/levelrail-agent's own main.go
// reads APP_NODE_KEEPALIVE_TIME/APP_NODE_KEEPALIVE_TIMEOUT and passes
// the parsed durations here, the same env-var-with-default convention
// WithHeartbeatInterval above follows.
func WithKeepalive(pingTime, timeout time.Duration) SessionOption {
	return func(c *sessionConfig) { c.keepaliveTime, c.keepaliveTimeout = pingTime, timeout }
}

// serveSession is RunSession's pure loop, split out so it's directly
// testable against a fake agentClientStream: reads incoming
// ControlMessage frames and dispatches each against rt, replying with
// the resulting AgentResponse (and any ProxiedEvent or ExecOutput frames
// a watch or an exec produces along the way). It also starts this
// agent's own heartbeatLoop, sending an unprompted Heartbeat frame every
// heartbeatInterval for as long as the stream stays open: this is what
// internal/agent.Server's mux.onHeartbeat callback actually keys
// last_seen_at freshness off now, not the stream merely existing.
//
// Each request is dispatched in its own goroutine, not handled
// sequentially in this loop: a slow operation (Create pulling a large
// image, in particular) must not stall Recv from processing other,
// unrelated, concurrent requests on the same connection. Exec's own
// frames are routed to ExecRelay instead, which never blocks this loop
// for the same reason. Send is serialized by sendMu, since a gRPC stream
// is not safe for concurrent Send calls, the identical reasoning mux.go's
// own sendMu already documents for the control-plane side of this same
// connection.
func serveSession(ctx context.Context, stream agentClientStream, rt docker.Runtime, builder BuildRunner, heartbeatInterval time.Duration, logger *slog.Logger) error {
	var sendMu sync.Mutex
	send := func(msg *agentpb.AgentMessage) {
		sendMu.Lock()
		defer sendMu.Unlock()
		if err := stream.Send(msg); err != nil {
			logger.Warn("agent: session: send failed", slog.String("error", err.Error()))
		}
	}
	emitEvent := func(ev *agentpb.ProxiedEvent) {
		send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Event{Event: ev}})
	}

	execs := NewExecRelay(rt, send)
	defer execs.CloseAll()

	builds := NewBuildRelay(builder, send)
	defer builds.CloseAll()

	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)
	go heartbeatLoop(send, heartbeatInterval, heartbeatDone)

	for {
		msg, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("agent: session: recv: %w", err)
		}
		switch p := msg.GetPayload().(type) {
		case *agentpb.ControlMessage_Request:
			req := p.Request
			if exec := req.GetExec(); exec != nil {
				execs.Start(ctx, req.GetRequestId(), exec)
				continue
			}
			if b := req.GetBuild(); b != nil {
				builds.Start(ctx, req.GetRequestId(), b)
				continue
			}
			go func() {
				resp := Execute(ctx, rt, req, emitEvent)
				send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: resp}})
			}()
		case *agentpb.ControlMessage_ExecInput:
			execs.Input(p.ExecInput)
		case *agentpb.ControlMessage_ExecCancel:
			execs.Cancel(p.ExecCancel.GetExecId())
		case *agentpb.ControlMessage_ExecCredit:
			execs.Credit(p.ExecCredit)
		case *agentpb.ControlMessage_ExecResize:
			execs.Resize(p.ExecResize)
		case *agentpb.ControlMessage_BuildInput:
			builds.Input(p.BuildInput)
		case *agentpb.ControlMessage_BuildCancel:
			builds.Cancel(p.BuildCancel.GetBuildId())
		case *agentpb.ControlMessage_BuildCredit:
			builds.Credit(p.BuildCredit)
		}
	}
}

// heartbeatLoop sends an unprompted Heartbeat frame via send every
// interval, until done is closed. Runs in its own goroutine for the
// lifetime of one serveSession call: send is already safe for concurrent
// use (serialized by serveSession's own sendMu), so this never
// coordinates with the main recv loop beyond that.
func heartbeatLoop(send func(*agentpb.AgentMessage), interval time.Duration, done <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Heartbeat{Heartbeat: &agentpb.Heartbeat{}}})
		}
	}
}
