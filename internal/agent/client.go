package agent

// Agent-side connection logic: DialEnroll for the join-token exchange, RunSession for the persistent stream.

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/agent/pinned"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Heartbeat frame interval. Control-plane last_seen_at freshness depends on it, not on the stream staying open.
const defaultHeartbeatInterval = 15 * time.Second

// HTTP/2 PING keepalive detects a frozen agent whose TCP connection stays open.
// Worst-case detection is about 20s, under the node heartbeat staleness timeout.
const (
	defaultKeepaliveTime    = 10 * time.Second
	defaultKeepaliveTimeout = 10 * time.Second
)

// Identity is the PEM cert, key and CA that an enrolled node reconnects with; cmd/levelrail-agent persists them.
type Identity struct {
	NodeID        string
	ClientCertPEM []byte
	ClientKeyPEM  []byte
	CACertPEM     []byte
}

// ErrCAFingerprintMismatch is returned by DialEnroll when the control
// plane's CA does not match the pinned fingerprint.
var ErrCAFingerprintMismatch = pinned.ErrMismatch

// errControlPlaneGoAway ends serveSession when a GoAway frame arrives, so
// a restarting control plane's bounded GracefulStop wait does not have to
// burn its own timeout on a stream the agent is willing to close itself.
var errControlPlaneGoAway = errors.New("agent: session: control plane is shutting down")

// EnrollOption configures DialEnroll.
type EnrollOption func(*enrollConfig)

type enrollConfig struct {
	caFingerprint string
	caPEM         []byte
}

// pinState is what the agent tells the control plane about how it verified
// the CA: the operator's fingerprint, the CA from a prior enrollment, or
// neither.
func (c enrollConfig) pinState() string {
	switch {
	case c.caFingerprint != "":
		return store.CAPinPinned
	case len(c.caPEM) > 0:
		return store.CAPinTrustedCA
	}
	return store.CAPinUnpinned
}

// WithPinnedCAFingerprint makes DialEnroll refuse any control plane whose
// CA certificate's SHA-256 (hex, colons optional) is not fingerprint.
func WithPinnedCAFingerprint(fingerprint string) EnrollOption {
	return func(c *enrollConfig) { c.caFingerprint = normalizeFingerprint(fingerprint) }
}

func normalizeFingerprint(fp string) string {
	return pinned.Normalize(fp)
}

// DialEnroll exchanges joinToken for an Identity over addr; the private key never leaves the agent.
func DialEnroll(ctx context.Context, addr, joinToken, nodeName string, opts ...EnrollOption) (*Identity, error) {
	var cfg enrollConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	keyPEM, csrDER, err := NewKeyAndCSR(nodeName)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(cfg.tlsConfig())))
	if err != nil {
		return nil, fmt.Errorf("agent: dial %q for enrollment: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := agentpb.NewAgentServiceClient(conn).Enroll(ctx, &agentpb.EnrollRequest{
		JoinToken:  joinToken,
		NodeName:   nodeName,
		CsrDer:     csrDER,
		Agent:      LocalAgentInfo(),
		CaPinState: cfg.pinState(),
	})
	if err != nil {
		return nil, fmt.Errorf("agent: enroll: %w", err)
	}
	if err := cfg.checkPinnedCA(resp.GetCaCertPem()); err != nil {
		return nil, fmt.Errorf("agent: enroll: %w", err)
	}
	if err := validateIssued(resp.GetNodeId(), resp.GetClientCertPem(), keyPEM, resp.GetCaCertPem(), time.Now()); err != nil {
		return nil, fmt.Errorf("agent: enroll: %w", err)
	}

	return &Identity{
		NodeID:        resp.GetNodeId(),
		ClientCertPEM: resp.GetClientCertPem(),
		ClientKeyPEM:  keyPEM,
		CACertPEM:     resp.GetCaCertPem(),
	}, nil
}

// DialReenroll exchanges a re-enrollment token for a new certificate for an
// existing node, keeping its identity. nodeID may be empty when the old
// identity file is gone; the token alone names the node.
func DialReenroll(ctx context.Context, addr, token, nodeID string, opts ...EnrollOption) (*Identity, error) {
	var cfg enrollConfig
	for _, opt := range opts {
		opt(&cfg)
	}
	keyPEM, csrDER, err := NewKeyAndCSR(nodeID)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(credentials.NewTLS(cfg.tlsConfig())))
	if err != nil {
		return nil, fmt.Errorf("agent: dial %q for re-enrollment: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	resp, err := agentpb.NewAgentServiceClient(conn).Reenroll(ctx, &agentpb.ReenrollRequest{
		ReenrollToken: token, NodeId: nodeID, CsrDer: csrDER, Agent: LocalAgentInfo(), CaPinState: cfg.pinState(),
	})
	if err != nil {
		return nil, fmt.Errorf("agent: re-enroll: %w", err)
	}
	if err := cfg.checkPinnedCA(resp.GetCaCertPem()); err != nil {
		return nil, fmt.Errorf("agent: re-enroll: %w", err)
	}
	if err := validateIssued(resp.GetNodeId(), resp.GetClientCertPem(), keyPEM, resp.GetCaCertPem(), time.Now()); err != nil {
		return nil, fmt.Errorf("agent: re-enroll: %w", err)
	}
	return &Identity{NodeID: resp.GetNodeId(), ClientCertPEM: resp.GetClientCertPem(), ClientKeyPEM: keyPEM, CACertPEM: resp.GetCaCertPem()}, nil
}

// WithTrustedCA verifies the control plane against caPEM, the CA an
// already-enrolled node has on disk, instead of a fingerprint pin.
func WithTrustedCA(caPEM []byte) EnrollOption {
	return func(c *enrollConfig) { c.caPEM = caPEM }
}

func (c enrollConfig) tlsConfig() *tls.Config {
	if len(c.caPEM) > 0 {
		pool := x509.NewCertPool()
		if pool.AppendCertsFromPEM(c.caPEM) {
			return &tls.Config{RootCAs: pool, NextProtos: []string{"h2"}}
		}
	}
	return enrollTrustConfig(c.caFingerprint)
}

func (c enrollConfig) checkPinnedCA(caPEM []byte) error {
	if c.caFingerprint == "" {
		return nil
	}
	got, err := certFingerprintFromPEM(caPEM)
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(got), []byte(c.caFingerprint)) != 1 {
		return fmt.Errorf("%w: response carries CA %s", ErrCAFingerprintMismatch, got)
	}
	return nil
}

// enrollTrustConfig builds the enrollment TLS config. Go's built-in
// verification is skipped because there is no root to give it yet;
// with a pin, verifyPinnedChain replaces it.
func enrollTrustConfig(caFingerprint string) *tls.Config {
	return pinned.EnrollTLSConfig(caFingerprint)
}

// verifyPinnedChain accepts the connection only if the presented chain
// includes the pinned CA and the leaf is a valid server certificate
// signed by it. Hostnames are not checked: the pin is the trust anchor.
func verifyPinnedChain(chain []*x509.Certificate, caFingerprint string) error {
	return pinned.VerifyChain(chain, caFingerprint)
}

// agentClientStream is the narrow Session RPC surface serveSession needs, so tests can fake it.
type agentClientStream interface {
	Send(*agentpb.AgentMessage) error
	Recv() (*agentpb.ControlMessage, error)
}

// RunSession serves one persistent Session stream until ctx is cancelled or the connection fails.
// It never retries; the caller owns reconnection.
func RunSession(ctx context.Context, addr string, id *Identity, rt docker.Runtime, logger *slog.Logger, opts ...SessionOption) error {
	if logger == nil {
		logger = slog.Default()
	}

	var cfg sessionConfig
	for _, opt := range opts {
		opt(&cfg)
	}

	if cfg.renewer != nil {
		id = cfg.renewer.Holder().Current()
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

	client := agentpb.NewAgentServiceClient(conn)
	stream, err := client.Session(ctx)
	if err != nil {
		return fmt.Errorf("agent: open session: %w", err)
	}
	if err := stream.Send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Hello{Hello: LocalAgentInfo()}}); err != nil {
		return fmt.Errorf("agent: send hello: %w", err)
	}
	var sessionStream agentClientStream = stream
	if cfg.renewer != nil {
		renewCtx, stopRenew := context.WithCancel(ctx)
		defer stopRenew()
		rs := &renewalStream{inner: stream, r: cfg.renewer}
		sessionStream = rs
		go cfg.renewer.Run(renewCtx, client)
		go reportRenewals(renewCtx, cfg.renewer, func(m *agentpb.AgentMessage) error { return rs.Send(m) })
	}
	if cfg.onboardingProbe != nil {
		send := sessionStream.Send
		if cfg.renewer == nil {
			ls := &lockedStream{inner: stream}
			sessionStream, send = ls, ls.Send
		}
		obCtx, stopOnboarding := context.WithCancel(ctx)
		defer stopOnboarding()
		go onboardingLoop(obCtx, cfg.onboardingProbe, cfg.onboardingEvery, send)
	}

	heartbeatInterval := defaultHeartbeatInterval
	if cfg.heartbeatInterval > 0 {
		heartbeatInterval = cfg.heartbeatInterval
	}

	if cfg.silence != nil {
		ctx = withSilence(ctx, cfg.silence)
		cfg.silence.SetConnected(true)
		defer cfg.silence.SetConnected(false)
	}

	return serveSession(WithFileRoot(ctx, cfg.fileRoot), sessionStream, rt, cfg.builder, cfg.mesh, cfg.meshNodeID, heartbeatInterval, cfg.gpuProbe, logger, cfg.hostProbe)
}

// sessionConfig holds RunSession's optional wiring.
type sessionConfig struct {
	builder           BuildRunner
	mesh              MeshApplier
	meshNodeID        string
	heartbeatInterval time.Duration
	keepaliveTime     time.Duration
	keepaliveTimeout  time.Duration
	gpuProbe          GPUProbe
	hostProbe         HostProbe
	onboardingProbe   OnboardingProbe
	onboardingEvery   time.Duration
	renewer           *Renewer
	fileRoot          string
	silence           *SilenceWatcher
}

// WithFileStorage lets the control plane read and write objects under dir, and nowhere else, over this session.
func WithFileStorage(dir string) SessionOption {
	return func(c *sessionConfig) { c.fileRoot = dir }
}

// WithRenewer keeps the session's certificate fresh with r while the
// session is open. The identity r holds replaces the one RunSession was
// given, so each reconnect presents the newest certificate.
func WithRenewer(r *Renewer) SessionOption {
	return func(c *sessionConfig) { c.renewer = r }
}

// SessionOption configures optional RunSession behavior.
type SessionOption func(*sessionConfig)

// WithBuildRunner sets the build runner; without one, dispatched builds are rejected up front.
func WithBuildRunner(runner BuildRunner) SessionOption {
	return func(c *sessionConfig) { c.builder = runner }
}

// WithMesh enables ApplyMesh and RotateMeshKey on this stream; without it both answer ErrMeshUnavailable.
func WithMesh(nodeID string, mesh MeshApplier) SessionOption {
	return func(c *sessionConfig) { c.meshNodeID, c.mesh = nodeID, mesh }
}

// WithHeartbeatInterval overrides the Heartbeat cadence; the env var is read by cmd/levelrail-agent, not here.
func WithHeartbeatInterval(d time.Duration) SessionOption {
	return func(c *sessionConfig) { c.heartbeatInterval = d }
}

// WithKeepalive overrides the HTTP/2 PING keepalive timing; the caller supplies env-derived durations.
func WithKeepalive(pingTime, timeout time.Duration) SessionOption {
	return func(c *sessionConfig) { c.keepaliveTime, c.keepaliveTimeout = pingTime, timeout }
}

// Each request runs in its own goroutine so one slow op (a large image pull) cannot stall the others.
// Sends go through sendMu: a gRPC stream is not safe for concurrent Send.
func serveSession(ctx context.Context, stream agentClientStream, rt docker.Runtime, builder BuildRunner, mesh MeshApplier, meshNodeID string, heartbeatInterval time.Duration, gpuProbe GPUProbe, logger *slog.Logger, hostProbes ...HostProbe) error {
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
	if gpuProbe != nil {
		go gpuReportLoop(ctx, send, gpuProbe, gpuReportInterval, heartbeatDone)
	}
	for _, hp := range hostProbes {
		if hp != nil {
			go hostReportLoop(ctx, send, hp, hostReportInterval, heartbeatDone)
		}
	}

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
			if am := req.GetApplyMesh(); am != nil {
				go handleApplyMesh(ctx, mesh, meshNodeID, req.GetRequestId(), am, send)
				continue
			}
			if req.GetRotateMeshKey() != nil {
				go handleRotateMeshKey(ctx, mesh, meshNodeID, req.GetRequestId(), send)
				continue
			}
			if req.GetMeshStatus() != nil {
				go handleMeshStatus(ctx, mesh, req.GetRequestId(), send)
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
		case *agentpb.ControlMessage_SilenceConfig:
			if w := silenceFrom(ctx); w != nil {
				w.SetURL(p.SilenceConfig.GetUrl())
			}
		case *agentpb.ControlMessage_GoAway:
			return errControlPlaneGoAway
		}
	}
}

// Sends a Heartbeat every interval until done closes; send is already serialized by sendMu.
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
