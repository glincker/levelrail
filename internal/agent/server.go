package agent

// This file: the control plane's agentpb.AgentServiceServer. Certificate
// renewal and re-enrollment live in server_cert.go (ADR 021).

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EnrollStore is the store surface Server needs. *store.DB satisfies it.
type EnrollStore interface {
	GetNodeJoinTokenByHash(ctx context.Context, hash string) (*store.NodeJoinToken, error)
	MarkNodeJoinTokenUsed(ctx context.Context, id string) error
	SaveNode(ctx context.Context, n store.Node) error
	GetNode(ctx context.Context, id string) (*store.Node, error)
	UpdateNodeStatus(ctx context.Context, id string, status store.NodeStatus) error
	TouchNodeLastSeen(ctx context.Context, id string) error
	RotateNodeCert(ctx context.Context, id, presentedFingerprint string, c store.NodeCert, grace time.Duration, now time.Time) error
	ReenrollNodeCert(ctx context.Context, id string, c store.NodeCert, now time.Time) error
	SyncNodeCertDetails(ctx context.Context, id, fingerprint, serial string, notAfter time.Time) error
	UpdateNodeAgentInfo(ctx context.Context, id string, info store.NodeAgentInfo, now time.Time) error
}

// Defaults for the env-tunable certificate settings (ADR 021).
const (
	DefaultClientCertValidity = 90 * 24 * time.Hour
	DefaultCertRenewGrace     = 24 * time.Hour
)

// Server implements agentpb.AgentServiceServer.
type Server struct {
	agentpb.UnimplementedAgentServiceServer
	ca       *CA
	store    EnrollStore
	registry *Registry
	logger   *slog.Logger
	gpuSink  GPUSink // nil is valid: GPU reports are ignored

	certValidity time.Duration
	renewGrace   time.Duration
	requireCSR   bool
	now          func() time.Time

	sessionsMu sync.Mutex
	sessions   map[string]*liveSession
}

type liveSession struct {
	kick chan struct{}
}

// Option configures optional Server behavior.
type Option func(*Server)

// WithCertValidity sets how long issued agent certificates stay valid.
func WithCertValidity(d time.Duration) Option {
	return func(s *Server) {
		if d > 0 {
			s.certValidity = d
		}
	}
}

// WithRenewGrace sets how long a renewed node's previous certificate stays
// accepted, so a renewal whose response was lost cannot lock the node out.
func WithRenewGrace(d time.Duration) Option {
	return func(s *Server) {
		if d > 0 {
			s.renewGrace = d
		}
	}
}

// WithRequireCSR refuses enrollments from agents that do not send a CSR,
// closing the legacy path where the control plane generates the key.
func WithRequireCSR(require bool) Option {
	return func(s *Server) { s.requireCSR = require }
}

// WithClock overrides the server's time source, for tests.
func WithClock(now func() time.Time) Option {
	return func(s *Server) { s.now = now }
}

// NewServer builds a Server. logger defaults to slog.Default() if nil.
func NewServer(ca *CA, st EnrollStore, registry *Registry, logger *slog.Logger, opts ...Option) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{
		ca: ca, store: st, registry: registry, logger: logger,
		certValidity: DefaultClientCertValidity, renewGrace: DefaultCertRenewGrace,
		now: time.Now, sessions: map[string]*liveSession{},
	}
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
	if len(req.GetCsrDer()) == 0 && s.requireCSR {
		return nil, status.Error(codes.FailedPrecondition, "this control plane requires agents to generate their own key: upgrade the agent")
	}

	if _, err := s.redeemToken(ctx, req.GetJoinToken(), store.NodeJoinTokenPurposeEnroll); err != nil {
		return nil, err
	}

	nodeID, err := randomNodeID()
	if err != nil {
		s.logger.Error("agent: enroll: generate node id failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}

	issued, keyPEM, origin, err := s.issueForEnroll(nodeID, req.GetCsrDer())
	if err != nil {
		return nil, err
	}

	now := s.now()
	if err := s.store.SaveNode(ctx, store.Node{
		ID: nodeID, Name: req.GetNodeName(), Status: store.NodeStatusPending,
		CertFingerprint: issued.Fingerprint, CertNotAfter: &issued.NotAfter, CertSerial: issued.Serial, CertKeyOrigin: origin,
		CreatedAt: now, UpdatedAt: now,
		AcceptsAppWorkloads: true,
	}); err != nil {
		if errors.Is(err, store.ErrNodeNameTaken) {
			return nil, status.Errorf(codes.AlreadyExists, "node name %q is already taken", req.GetNodeName())
		}
		s.logger.Error("agent: enroll: save node failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	s.recordAgentInfo(ctx, nodeID, req.GetAgent())

	s.logger.Info("agent: node enrolled", slog.String("node_id", nodeID), slog.String("name", req.GetNodeName()), slog.String("key_origin", origin))

	return &agentpb.EnrollResponse{
		NodeId:        nodeID,
		ClientCertPem: issued.PEM,
		ClientKeyPem:  keyPEM,
		CaCertPem:     s.ca.CertPEM(),
		NotAfter:      timestamppb.New(issued.NotAfter),
	}, nil
}

// issueForEnroll signs the agent's CSR, or for an agent too old to send
// one, generates the key server-side and returns it (the pre-ADR 021 path).
func (s *Server) issueForEnroll(nodeID string, csrDER []byte) (IssuedCert, []byte, string, error) {
	if len(csrDER) > 0 {
		issued, err := s.signCSR(nodeID, csrDER)
		return issued, nil, store.CertKeyOriginAgent, err
	}
	s.logger.Warn("agent: legacy enrollment without a CSR, generating the key on the control plane", slog.String("node_id", nodeID))
	certPEM, keyPEM, err := s.ca.IssueClientCert(nodeID, s.certValidity)
	if err != nil {
		s.logger.Error("agent: issue client cert failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return IssuedCert{}, nil, "", status.Error(codes.Internal, "internal error")
	}
	cert, err := parseCertPEM(certPEM)
	if err != nil {
		s.logger.Error("agent: parse issued cert failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return IssuedCert{}, nil, "", status.Error(codes.Internal, "internal error")
	}
	issued, err := issuedFromDER(cert.Raw)
	if err != nil {
		s.logger.Error("agent: parse issued cert failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return IssuedCert{}, nil, "", status.Error(codes.Internal, "internal error")
	}
	return issued, keyPEM, store.CertKeyOriginServer, nil
}

func (s *Server) signCSR(nodeID string, csrDER []byte) (IssuedCert, error) {
	issued, err := s.ca.SignClientCSR(nodeID, csrDER, s.certValidity, s.now())
	if errors.Is(err, ErrInvalidCSR) {
		return IssuedCert{}, status.Errorf(codes.InvalidArgument, "%v", err)
	}
	if err != nil {
		s.logger.Error("agent: sign client CSR failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return IssuedCert{}, status.Error(codes.Internal, "internal error")
	}
	return issued, nil
}

// redeemToken validates plaintext as an unused, unexpired token of purpose
// and consumes it. The token is spent before anything else irreversible
// happens, so a later failure never gives the same token a second try.
func (s *Server) redeemToken(ctx context.Context, plaintext, purpose string) (*store.NodeJoinToken, error) {
	tok, err := s.store.GetNodeJoinTokenByHash(ctx, hashJoinToken(plaintext))
	if errors.Is(err, store.ErrNodeJoinTokenNotFound) {
		return nil, status.Error(codes.Unauthenticated, "invalid join token")
	}
	if err != nil {
		s.logger.Error("agent: look up join token failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	if got := tokenPurpose(tok); got != purpose {
		return nil, status.Errorf(codes.Unauthenticated, "this is a %s token, not a %s token", got, purpose)
	}
	if tok.UsedAt != nil {
		return nil, status.Error(codes.Unauthenticated, "join token already used")
	}
	if s.now().After(tok.ExpiresAt) {
		return nil, status.Error(codes.Unauthenticated, "join token expired")
	}
	if err := s.store.MarkNodeJoinTokenUsed(ctx, tok.ID); err != nil {
		if errors.Is(err, store.ErrNodeJoinTokenAlreadyUsed) {
			return nil, status.Error(codes.Unauthenticated, "join token already used")
		}
		s.logger.Error("agent: mark join token used failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	return tok, nil
}

func tokenPurpose(t *store.NodeJoinToken) string {
	if t.Purpose == "" {
		return store.NodeJoinTokenPurposeEnroll
	}
	return t.Purpose
}

// Session implements agentpb.AgentServiceServer: accepts an
// mTLS-authenticated agent's persistent stream once its certificate is
// confirmed to be one this node may use, and wires a GRPCTransport into
// Registry until the stream ends.
func (s *Server) Session(stream agentpb.AgentService_SessionServer) error {
	ctx := stream.Context()
	node, peerCert, err := s.authenticatePeer(ctx)
	if err != nil {
		return err
	}
	nodeID := node.ID
	if fp := CertFingerprint(peerCert.Raw); fp == node.CertFingerprint {
		if err := s.store.SyncNodeCertDetails(ctx, nodeID, fp, certSerial(peerCert), peerCert.NotAfter); err != nil {
			s.logger.Warn("agent: session: sync cert details failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		}
	}

	if err := s.store.UpdateNodeStatus(ctx, nodeID, store.NodeStatusOnline); err != nil {
		s.logger.Warn("agent: session: update node status failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
	}
	if err := s.store.TouchNodeLastSeen(ctx, nodeID); err != nil {
		s.logger.Warn("agent: session: touch last seen failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
	}

	// last_seen_at advances only on real Heartbeat frames, so a hung agent
	// with an open connection stops looking healthy. Buffered and
	// non-blocking: recvLoop must never block delivering the signal.
	heartbeats := make(chan struct{}, 1)
	m := newMuxWithHandlers(stream, muxHandlers{
		gpu: s.onGPUReport(nodeID),
		heartbeat: func() {
			select {
			case heartbeats <- struct{}{}:
			default:
			}
		},
		hello: func(info *agentpb.AgentInfo) {
			go s.recordAgentInfo(context.Background(), nodeID, info)
		},
	})
	s.registry.Register(nodeID, newGRPCTransport(m))
	live := s.trackSession(nodeID)
	s.logger.Info("agent: node connected", slog.String("node_id", nodeID))

	heartbeatDone := make(chan struct{})
	go s.heartbeatLoop(nodeID, heartbeats, heartbeatDone)

	defer func() {
		close(heartbeatDone)
		s.untrackSession(nodeID, live)
		s.registry.Unregister(nodeID)
		// The stream's context is already done here; the offline mark
		// must still reach the store.
		if err := s.store.UpdateNodeStatus(context.Background(), nodeID, store.NodeStatusOffline); err != nil {
			s.logger.Warn("agent: session: mark node offline failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		}
		s.logger.Info("agent: node disconnected", slog.String("node_id", nodeID))
	}()

	select {
	case <-m.closed:
		return nil
	case <-live.kick:
		return status.Error(codes.PermissionDenied, "session closed by the control plane: node certificate revoked")
	}
}

// authenticatePeer resolves the caller's client certificate to the node it
// may act as, rejecting unknown nodes, revoked certificates, and any
// certificate that is neither current nor inside the renewal overlap.
func (s *Server) authenticatePeer(ctx context.Context) (*store.Node, *x509.Certificate, error) {
	cert, err := peerCertificate(ctx)
	if err != nil {
		return nil, nil, status.Errorf(codes.Unauthenticated, "%v", err)
	}
	nodeID := cert.Subject.CommonName
	node, err := s.store.GetNode(ctx, nodeID)
	if errors.Is(err, store.ErrNodeNotFound) {
		return nil, nil, status.Errorf(codes.Unauthenticated, "unknown node %q", nodeID)
	}
	if err != nil {
		s.logger.Error("agent: look up node failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return nil, nil, status.Error(codes.Internal, "internal error")
	}
	if node.CertRevokedAt != nil {
		return nil, nil, status.Errorf(codes.PermissionDenied, "certificate for node %q was revoked: re-enroll the node", nodeID)
	}
	if !node.AcceptsCert(CertFingerprint(cert.Raw), s.now()) {
		return nil, nil, status.Errorf(codes.Unauthenticated, "certificate fingerprint mismatch for node %q", nodeID)
	}
	return node, cert, nil
}

// Disconnect ends nodeID's live Session, if any, so a node whose
// certificate was just revoked stops acting on an already-open stream.
func (s *Server) Disconnect(nodeID string) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	if live, ok := s.sessions[nodeID]; ok {
		close(live.kick)
		delete(s.sessions, nodeID)
	}
}

func (s *Server) trackSession(nodeID string) *liveSession {
	live := &liveSession{kick: make(chan struct{})}
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	s.sessions[nodeID] = live
	return live
}

func (s *Server) untrackSession(nodeID string, live *liveSession) {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	if s.sessions[nodeID] == live {
		delete(s.sessions, nodeID)
	}
}

func (s *Server) recordAgentInfo(ctx context.Context, nodeID string, info *agentpb.AgentInfo) {
	if info == nil {
		return
	}
	if err := s.store.UpdateNodeAgentInfo(ctx, nodeID, store.NodeAgentInfo{
		Version: info.GetVersion(), Commit: info.GetCommit(), OS: info.GetOs(), Arch: info.GetArch(),
	}, s.now()); err != nil {
		s.logger.Warn("agent: store agent info failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
	}
}

// onGPUReport returns the mux callback persisting nodeID's GPU reports,
// or nil when no sink is configured. It runs on the stream's recv
// goroutine, so the write happens off it.
func (s *Server) onGPUReport(nodeID string) func(*agentpb.GPUReport) {
	if s.gpuSink == nil {
		return nil
	}
	return func(r *agentpb.GPUReport) {
		info := gpuInfoFromPB(r)
		go func() {
			if err := s.gpuSink.SetNodeGPU(context.Background(), nodeID, info); err != nil {
				s.logger.Warn("agent: session: store gpu report failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			}
		}()
	}
}

// heartbeatLoop touches last_seen_at for nodeID each time a Heartbeat frame
// arrives, until done is closed. A failed touch is only logged: the next
// heartbeat is the retry.
func (s *Server) heartbeatLoop(nodeID string, heartbeats <-chan struct{}, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case <-heartbeats:
			if err := s.store.TouchNodeLastSeen(context.Background(), nodeID); err != nil {
				s.logger.Warn("agent: session: heartbeat touch failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
			}
		}
	}
}

// peerCertificate returns the verified client certificate on ctx's
// connection. Its CommonName is the node ID it was issued to.
func peerCertificate(ctx context.Context) (*x509.Certificate, error) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return nil, fmt.Errorf("no peer TLS info on this connection")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, fmt.Errorf("peer auth info is %T, want TLS", p.AuthInfo)
	}
	if len(tlsInfo.State.PeerCertificates) == 0 {
		return nil, fmt.Errorf("no client certificate presented")
	}
	return tlsInfo.State.PeerCertificates[0], nil
}

func hashJoinToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

func certFingerprintFromPEM(certPEM []byte) (string, error) {
	cert, err := parseCertPEM(certPEM)
	if err != nil {
		return "", err
	}
	return CertFingerprint(cert.Raw), nil
}

// randomNodeID reuses randomRequestID's generator: a node ID is the
// certificate's CommonName, so it only needs to be unique and unguessable.
func randomNodeID() (string, error) {
	return randomRequestID()
}
