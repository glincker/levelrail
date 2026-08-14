package agent

// This file: the control plane's own implementation of
// agentpb.AgentServiceServer (TASKS.md 3.2). Enroll validates a
// TASKS.md 3.1 join token and issues a client certificate (pki.go);
// Session accepts an already-mTLS-authenticated agent's persistent
// stream, confirms its certificate actually matches the node it claims
// to be, and wires up a GRPCTransport into Registry (transport.go) for
// the rest of the control plane to use until the stream ends.

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EnrollStore is the narrow store surface Server needs: validate and
// consume a join token (TASKS.md 3.1), persist the newly enrolled node,
// and track its connection state. *store.DB satisfies this
// structurally.
type EnrollStore interface {
	GetNodeJoinTokenByHash(ctx context.Context, hash string) (*store.NodeJoinToken, error)
	MarkNodeJoinTokenUsed(ctx context.Context, id string) error
	SaveNode(ctx context.Context, n store.Node) error
	GetNode(ctx context.Context, id string) (*store.Node, error)
	UpdateNodeStatus(ctx context.Context, id string, status store.NodeStatus) error
	TouchNodeLastSeen(ctx context.Context, id string) error
}

// clientCertValidity is how long an issued agent certificate stays
// valid. ADR 003's Consequences section names "certificate rotation on
// a schedule" as real Phase 3 scope; the rotation mechanism itself
// (renewing or re-enrolling before expiry) is not built in this pass,
// only the expiry is set here, deliberately short enough (90 days, not
// this package's own CA's 10-year validity) that an unbuilt rotation
// mechanism is a visible, real gap rather than one nobody would notice
// for a year.
const clientCertValidity = 90 * 24 * time.Hour

// defaultHeartbeatInterval is how often Session touches last_seen_at
// for a connected node while its stream stays open (TASKS.md 3.7),
// matching CLAUDE.md 4.8's own metrics-collection cadence (15s): reusing
// that same number isn't required by anything, but two independent "how
// fresh does this need to be" judgment calls landing on the same value
// is a reasonable default rather than inventing a third cadence.
const defaultHeartbeatInterval = 15 * time.Second

// Server implements agentpb.AgentServiceServer.
type Server struct {
	agentpb.UnimplementedAgentServiceServer
	ca                *CA
	store             EnrollStore
	registry          *Registry
	logger            *slog.Logger
	heartbeatInterval time.Duration
}

// Option configures optional Server behavior.
type Option func(*Server)

// WithHeartbeatInterval overrides how often Session touches
// last_seen_at for a connected node. Without one configured,
// defaultHeartbeatInterval applies. cmd/levelrail's own main.go reads
// APP_NODE_HEARTBEAT_INTERVAL and passes the parsed duration here
// (CLAUDE.md 7: "no hardcoded thresholds, use env vars"); this package
// itself never reads the environment directly, the same "constructor
// args only" convention api.WithSessionTTL already establishes.
func WithHeartbeatInterval(d time.Duration) Option {
	return func(s *Server) { s.heartbeatInterval = d }
}

// NewServer builds a Server. logger defaults to slog.Default() if nil.
func NewServer(ca *CA, st EnrollStore, registry *Registry, logger *slog.Logger, opts ...Option) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{ca: ca, store: st, registry: registry, logger: logger, heartbeatInterval: defaultHeartbeatInterval}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Enroll implements agentpb.AgentServiceServer.
func (s *Server) Enroll(ctx context.Context, req *agentpb.EnrollRequest) (*agentpb.EnrollResponse, error) {
	if req.GetJoinToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "join_token is required")
	}
	if req.GetNodeName() == "" {
		return nil, status.Error(codes.InvalidArgument, "node_name is required")
	}

	tok, err := s.store.GetNodeJoinTokenByHash(ctx, hashJoinToken(req.GetJoinToken()))
	if errors.Is(err, store.ErrNodeJoinTokenNotFound) {
		return nil, status.Error(codes.Unauthenticated, "invalid join token")
	}
	if err != nil {
		s.logger.Error("agent: enroll: look up join token failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	if tok.UsedAt != nil {
		return nil, status.Error(codes.Unauthenticated, "join token already used")
	}
	if time.Now().After(tok.ExpiresAt) {
		return nil, status.Error(codes.Unauthenticated, "join token expired")
	}

	// Consume the token before doing anything else irreversible: a
	// failure past this point (e.g. SaveNode losing a name race) still
	// leaves the token spent, the same "fail safe over allowing a
	// one-time credential a second attempt" preference this codebase's
	// other single-use resources already follow.
	if err := s.store.MarkNodeJoinTokenUsed(ctx, tok.ID); err != nil {
		if errors.Is(err, store.ErrNodeJoinTokenAlreadyUsed) {
			return nil, status.Error(codes.Unauthenticated, "join token already used")
		}
		s.logger.Error("agent: enroll: mark join token used failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}

	nodeID, err := randomNodeID()
	if err != nil {
		s.logger.Error("agent: enroll: generate node id failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}

	certPEM, keyPEM, err := s.ca.IssueClientCert(nodeID, clientCertValidity)
	if err != nil {
		s.logger.Error("agent: enroll: issue client cert failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	fingerprint, err := certFingerprintFromPEM(certPEM)
	if err != nil {
		s.logger.Error("agent: enroll: compute cert fingerprint failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}

	now := time.Now()
	if err := s.store.SaveNode(ctx, store.Node{
		ID: nodeID, Name: req.GetNodeName(), Status: store.NodeStatusPending,
		CertFingerprint: fingerprint, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		if errors.Is(err, store.ErrNodeNameTaken) {
			return nil, status.Errorf(codes.AlreadyExists, "node name %q is already taken", req.GetNodeName())
		}
		s.logger.Error("agent: enroll: save node failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}

	s.logger.Info("agent: node enrolled", slog.String("node_id", nodeID), slog.String("name", req.GetNodeName()))

	return &agentpb.EnrollResponse{
		NodeId:        nodeID,
		ClientCertPem: certPEM,
		ClientKeyPem:  keyPEM,
		CaCertPem:     s.ca.CertPEM(),
	}, nil
}

// Session implements agentpb.AgentServiceServer: accepts an
// mTLS-authenticated agent's persistent stream, confirms its
// certificate actually matches the node it claims to be (not just that
// *some* certificate this CA issued was presented: a compromised or
// misconfigured node presenting a different, still-CA-issued
// certificate for another node's ID must not be trusted as that other
// node), and wires up a GRPCTransport into Registry for the rest of the
// control plane to use until the stream ends.
func (s *Server) Session(stream agentpb.AgentService_SessionServer) error {
	ctx := stream.Context()
	nodeID, fingerprint, err := peerIdentity(ctx)
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "session: %v", err)
	}

	node, err := s.store.GetNode(ctx, nodeID)
	if errors.Is(err, store.ErrNodeNotFound) {
		return status.Errorf(codes.Unauthenticated, "session: unknown node %q", nodeID)
	}
	if err != nil {
		s.logger.Error("agent: session: look up node failed", slog.String("error", err.Error()))
		return status.Error(codes.Internal, "internal error")
	}
	if node.CertFingerprint != fingerprint {
		return status.Errorf(codes.Unauthenticated, "session: certificate fingerprint mismatch for node %q", nodeID)
	}

	if err := s.store.UpdateNodeStatus(ctx, nodeID, store.NodeStatusOnline); err != nil {
		s.logger.Warn("agent: session: update node status failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
	}
	if err := s.store.TouchNodeLastSeen(ctx, nodeID); err != nil {
		s.logger.Warn("agent: session: touch last seen failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
	}

	m := newMux(stream)
	s.registry.Register(nodeID, newGRPCTransport(m))
	s.logger.Info("agent: node connected", slog.String("node_id", nodeID))

	// heartbeatDone stops the periodic TouchNodeLastSeen loop below
	// (TASKS.md 3.7): a single touch at connect time (above) can't
	// distinguish "still connected" from "connected an hour ago, then
	// the process hung", which is exactly what
	// internal/reconcile/nodehealth needs LastSeenAt to reflect.
	heartbeatDone := make(chan struct{})
	go s.heartbeatLoop(nodeID, heartbeatDone)

	defer func() {
		close(heartbeatDone)
		s.registry.Unregister(nodeID)
		// context.Background(), not ctx: the stream's own context is
		// already done by the time this defer runs (that's why Session
		// is returning), and this update needs to actually reach the
		// store, not be cancelled along with it.
		if err := s.store.UpdateNodeStatus(context.Background(), nodeID, store.NodeStatusOffline); err != nil {
			s.logger.Warn("agent: session: mark node offline failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		}
		s.logger.Info("agent: node disconnected", slog.String("node_id", nodeID))
	}()

	<-m.closed
	return nil
}

// heartbeatLoop touches last_seen_at for nodeID every heartbeatInterval
// until done is closed. Runs in its own goroutine for the lifetime of
// one Session call; a single TouchNodeLastSeen failure is logged and
// retried on the next tick, the same "best-effort, never block the
// caller" posture TouchNodeLastSeen's own doc comment already commits
// to.
func (s *Server) heartbeatLoop(nodeID string, done <-chan struct{}) {
	ticker := time.NewTicker(s.heartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			// context.Background(), not the stream's context: the same
			// reasoning as the final offline update above, a heartbeat
			// touch must not be cancelled by the stream context tearing
			// down mid-tick.
			if err := s.store.TouchNodeLastSeen(context.Background(), nodeID); err != nil {
				s.logger.Warn("agent: session: heartbeat touch failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			}
		}
	}
}

// peerIdentity extracts the enrolled node's ID (the mTLS client
// certificate's CommonName, set to the node ID at issuance by
// IssueClientCert in Enroll above) and that certificate's own
// fingerprint from ctx's gRPC peer info.
func peerIdentity(ctx context.Context) (nodeID, fingerprint string, err error) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return "", "", fmt.Errorf("no peer TLS info on this connection")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return "", "", fmt.Errorf("peer auth info is %T, want TLS", p.AuthInfo)
	}
	if len(tlsInfo.State.PeerCertificates) == 0 {
		return "", "", fmt.Errorf("no client certificate presented")
	}
	cert := tlsInfo.State.PeerCertificates[0]
	return cert.Subject.CommonName, CertFingerprint(cert.Raw), nil
}

func hashJoinToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func certFingerprintFromPEM(certPEM []byte) (string, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return "", fmt.Errorf("agent: no PEM block found in certificate data")
	}
	if _, err := x509.ParseCertificate(block.Bytes); err != nil {
		return "", fmt.Errorf("agent: parse certificate: %w", err)
	}
	return CertFingerprint(block.Bytes), nil
}

func randomNodeID() (string, error) {
	// Reuses the same generation shape as randomRequestID
	// (crypto/rand, hex-encoded), a different, purpose-specific
	// function rather than a shared one: a node ID is long-lived and
	// externally visible (it's the certificate's own CommonName), a
	// request ID is ephemeral and internal, different enough concerns
	// that conflating their generators would be the wrong kind of reuse.
	return randomRequestID()
}
