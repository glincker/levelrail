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
	"regexp"
	"sync"
	"sync/atomic"
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
	ca             *CA
	store          EnrollStore
	registry       *Registry
	logger         *slog.Logger
	gpuSink        GPUSink // nil is valid: GPU reports are ignored
	silenceSrc     atomic.Pointer[SilenceSource]
	hostSink       HostReportSink // nil is valid: host reports are ignored
	onboardingSink OnboardingSink // nil is valid: onboarding reports are ignored
	onSession      func(nodeID, remoteIP string)

	certValidity time.Duration
	renewGrace   time.Duration
	now          func() time.Time

	sessionsMu sync.Mutex
	sessions   map[string]*liveSession

	caPool   *x509.CertPool
	secStore SecurityStore
	secCfg   SecurityConfig
	sec      *securityRecorder
}

type liveSession struct {
	kick chan struct{}
	m    *mux
	once sync.Once
	// superseded is set, before kick closes, when a newer connection from the same node replaced this one.
	superseded bool
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
	s.initSecurity()
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
	if !validEnrollNodeName(req.GetNodeName()) {
		return nil, status.Error(codes.InvalidArgument, "node_name must be 1-63 characters of letters, digits, dot, underscore or hyphen")
	}
	if len(req.GetCsrDer()) == 0 {
		s.recordUpgradeRequired(ctx, store.SecurityKindEnroll, "")
		return nil, errAgentUpgradeRequired
	}

	tok, err := s.redeemToken(ctx, req.GetJoinToken(), store.NodeJoinTokenPurposeEnroll)
	if err != nil {
		s.recordTokenRefusal(ctx, store.SecurityKindEnroll, "", err)
		return nil, err
	}

	enrolled := false
	defer func() {
		if !enrolled {
			s.releaseToken(tok.ID)
		}
	}()

	nodeID, err := randomNodeID()
	if err != nil {
		s.logger.Error("agent: enroll: generate node id failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}

	issued, err := s.signCSR(nodeID, req.GetCsrDer())
	if err != nil {
		return nil, err
	}

	now := s.now()
	if err := s.store.SaveNode(ctx, store.Node{
		ID: nodeID, Name: req.GetNodeName(), Status: store.NodeStatusPending,
		CertFingerprint: issued.Fingerprint, CertNotAfter: &issued.NotAfter, CertSerial: issued.Serial, CertKeyOrigin: store.CertKeyOriginAgent,
		CreatedAt: now, UpdatedAt: now,
		AcceptsAppWorkloads: true,
	}); err != nil {
		if errors.Is(err, store.ErrNodeNameTaken) {
			return nil, status.Errorf(codes.AlreadyExists, "node name %q is already taken", req.GetNodeName())
		}
		s.logger.Error("agent: enroll: save node failed", slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	enrolled = true
	s.recordAgentInfo(ctx, nodeID, req.GetAgent())
	s.recordEnrolled(ctx, store.SecurityKindEnroll, nodeID, tok.ID, req.GetCaPinState(), issued)

	s.logger.Info("agent: node enrolled", slog.String("node_id", nodeID), slog.String("name", req.GetNodeName()))

	return &agentpb.EnrollResponse{
		NodeId:        nodeID,
		ClientCertPem: issued.PEM,
		CaCertPem:     s.ca.CertPEM(),
		NotAfter:      timestamppb.New(issued.NotAfter),
	}, nil
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

type tokenReleaser interface {
	ReleaseNodeJoinToken(ctx context.Context, id string) error
}

// releaseToken un-spends a join token when enrollment failed after redeeming it (name collision, CSR or store error), so the operator can retry with the same token.
func (s *Server) releaseToken(id string) {
	r, ok := s.store.(tokenReleaser)
	if !ok {
		return
	}
	if err := r.ReleaseNodeJoinToken(context.Background(), id); err != nil {
		s.logger.Error("agent: enroll: release join token failed", slog.String("token_id", id), slog.String("error", err.Error()))
	}
}

func tokenPurpose(t *store.NodeJoinToken) string {
	if t.Purpose == "" {
		return store.NodeJoinTokenPurposeEnroll
	}
	return t.Purpose
}

// Session implements agentpb.AgentServiceServer for an mTLS-authenticated agent stream.
func (s *Server) Session(stream agentpb.AgentService_SessionServer) error {
	ctx := stream.Context()
	node, peerCert, err := s.authenticatePeer(ctx)
	if err != nil {
		return err
	}
	nodeID := node.ID
	s.recordAccepted(ctx, nodeID, peerCert)
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
	caps := &capSet{}
	m := newMuxWithHandlers(stream, muxHandlers{
		gpu: s.onGPUReport(nodeID),
		hostReport: func(r *agentpb.HostReport) {
			if s.hostSink != nil {
				s.hostSink.SetNodeHostReport(nodeID, hostReportFromPB(r))
			}
		},
		onboarding: func(r *agentpb.OnboardingReport) {
			if s.onboardingSink != nil {
				s.onboardingSink.SetNodeOnboarding(nodeID, onboardingFromPB(r), s.now())
			}
		},
		heartbeat: func() {
			select {
			case heartbeats <- struct{}{}:
			default:
			}
		},
		hello: func(info *agentpb.AgentInfo) {
			caps.store(info.GetCapabilities())
			go s.recordAgentInfo(context.Background(), nodeID, info)
		},
		renewal: func(r *agentpb.RenewalReport) {
			go s.recordRenewalReport(context.Background(), nodeID, r)
		},
	})
	live := s.trackSession(nodeID, m)
	s.registry.Register(nodeID, newGRPCTransportWithCaps(m, caps))
	s.observeSession(ctx, nodeID)
	s.logger.Info("agent: node connected", slog.String("node_id", nodeID))

	heartbeatDone := make(chan struct{})
	go s.heartbeatLoop(nodeID, heartbeats, heartbeatDone)
	go s.silenceLoop(ctx, nodeID, m, silencePushEveryFromEnv(), heartbeatDone)

	defer func() {
		close(heartbeatDone)
		if !s.untrackSession(nodeID, live) {
			s.logger.Info("agent: superseded session ended", slog.String("node_id", nodeID))
			return
		}
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
		if live.superseded {
			return status.Error(codes.Aborted, "session replaced by a newer connection from this node")
		}
		return status.Error(codes.PermissionDenied, "session closed by the control plane: node certificate revoked")
	}
}

// authenticatePeer resolves the caller's client certificate to the node it
// may act as, rejecting unknown nodes, revoked certificates, and any
// certificate that is neither current nor inside the renewal overlap.
func (s *Server) authenticatePeer(ctx context.Context) (*store.Node, *x509.Certificate, error) {
	chain, err := peerChain(ctx)
	if err != nil {
		s.recordRejected(ctx, "", "", store.SecurityReasonNoCertificate, "")
		return nil, nil, status.Errorf(codes.Unauthenticated, "%v", err)
	}
	cert := chain[0]
	nodeID := cert.Subject.CommonName
	fp := CertFingerprint(cert.Raw)
	// The handshake already verified the chain; checking again keeps this
	// decision independent of how the listener is configured.
	if verr := verifyClientChain(chain, s.caPool, s.now()); verr != nil {
		s.recordRejected(ctx, "", nodeID, rejectReason(verr, s.now()), fp)
		return nil, nil, status.Errorf(codes.Unauthenticated, "client certificate refused")
	}
	node, err := s.store.GetNode(ctx, nodeID)
	if errors.Is(err, store.ErrNodeNotFound) {
		s.recordRejected(ctx, "", nodeID, store.SecurityReasonUnknownNode, fp)
		return nil, nil, status.Errorf(codes.Unauthenticated, "unknown node %q", nodeID)
	}
	if err != nil {
		s.logger.Error("agent: look up node failed", slog.String("node_id", nodeID), slog.String("error", err.Error()))
		return nil, nil, status.Error(codes.Internal, "internal error")
	}
	if node.CertRevokedAt != nil {
		s.recordRejected(ctx, nodeID, nodeID, store.SecurityReasonRevoked, fp)
		return nil, nil, status.Errorf(codes.PermissionDenied, "certificate for node %q was revoked: re-enroll the node", nodeID)
	}
	if !node.AcceptsCert(fp, s.now()) {
		s.recordRejected(ctx, nodeID, nodeID, store.SecurityReasonFingerprintMismatch, fp)
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
		live.end(false)
	}
}

func (l *liveSession) end(superseded bool) {
	l.once.Do(func() {
		l.superseded = superseded
		close(l.kick)
	})
}

func (s *Server) trackSession(nodeID string, m *mux) *liveSession {
	live := &liveSession{kick: make(chan struct{}), m: m}
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	if old, ok := s.sessions[nodeID]; ok {
		old.end(true)
		s.logger.Warn("agent: duplicate connection, closing the older session", slog.String("node_id", nodeID))
	}
	s.sessions[nodeID] = live
	return live
}

// NotifyShutdown sends GoAway to every agent, best-effort; agents that miss it fall back to the GracefulStop timeout.
func (s *Server) NotifyShutdown() {
	s.sessionsMu.Lock()
	muxes := make([]*mux, 0, len(s.sessions))
	for _, live := range s.sessions {
		muxes = append(muxes, live.m)
	}
	s.sessionsMu.Unlock()

	goAway := &agentpb.ControlMessage{Payload: &agentpb.ControlMessage_GoAway{GoAway: &agentpb.GoAway{}}}
	for _, m := range muxes {
		if err := m.sendFrame(goAway); err != nil {
			s.logger.Warn("agent: notify shutdown: send go-away failed", slog.String("error", err.Error()))
		}
	}
}

// untrackSession reports whether live was still the node's current session.
func (s *Server) untrackSession(nodeID string, live *liveSession) bool {
	s.sessionsMu.Lock()
	defer s.sessionsMu.Unlock()
	if s.sessions[nodeID] == live {
		delete(s.sessions, nodeID)
		return true
	}
	return false
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

// peerChain returns the client certificate chain on ctx's connection,
// leaf first. The leaf's CommonName is the node ID it was issued to.
func peerChain(ctx context.Context) ([]*x509.Certificate, error) {
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
	return tlsInfo.State.PeerCertificates, nil
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

var enrollNodeNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

func validEnrollNodeName(name string) bool { return enrollNodeNameRe.MatchString(name) }
