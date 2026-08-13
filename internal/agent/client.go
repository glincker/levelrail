package agent

// This file: the agent-side connection logic (TASKS.md 3.2): DialEnroll
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

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/docker"
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
// server-side, shown once, single-use, TASKS.md 3.1) is what actually
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
// that's cmd/levelrail-agent's own reconnect loop's job (TASKS.md 3.2's
// remaining wiring, and ADR 003's Consequences section's own "real,
// tested" reconnection/backpressure/version-negotiation requirement),
// kept out of this function so it stays a single, directly testable
// connection attempt rather than a policy about how many times or how
// fast to retry.
func RunSession(ctx context.Context, addr string, id *Identity, rt docker.Runtime, logger *slog.Logger) error {
	if logger == nil {
		logger = slog.Default()
	}

	cert, err := tls.X509KeyPair(id.ClientCertPEM, id.ClientKeyPEM)
	if err != nil {
		return fmt.Errorf("agent: parse identity certificate: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(id.CACertPEM) {
		return fmt.Errorf("agent: parse CA certificate: no valid certificate found")
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      pool,
		NextProtos:   []string{"h2"},
	})))
	if err != nil {
		return fmt.Errorf("agent: dial %q: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	stream, err := agentpb.NewAgentServiceClient(conn).Session(ctx)
	if err != nil {
		return fmt.Errorf("agent: open session: %w", err)
	}

	return serveSession(ctx, stream, rt, logger)
}

// serveSession is RunSession's pure loop, split out so it's directly
// testable against a fake agentClientStream: reads incoming
// ControlMessage (AgentRequest) frames and dispatches each via Execute
// against rt, replying with the resulting AgentResponse (and any
// ProxiedEvent frames a WatchEvents request's relay goroutine produces
// along the way).
//
// Each request is dispatched in its own goroutine, not handled
// sequentially in this loop: a slow operation (Create pulling a large
// image, in particular) must not stall Recv from processing other,
// unrelated, concurrent requests on the same connection. Send is
// serialized by sendMu, since a gRPC stream is not safe for concurrent
// Send calls, the identical reasoning mux.go's own sendMu already
// documents for the control-plane side of this same connection.
func serveSession(ctx context.Context, stream agentClientStream, rt docker.Runtime, logger *slog.Logger) error {
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

	for {
		msg, err := stream.Recv()
		if err != nil {
			return fmt.Errorf("agent: session: recv: %w", err)
		}
		req := msg.GetRequest()
		go func() {
			resp := Execute(ctx, rt, req, emitEvent)
			send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Response{Response: resp}})
		}()
	}
}
