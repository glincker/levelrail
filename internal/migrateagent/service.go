package migrateagent

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/agent/migrationpb"
	"github.com/GLINCKER/levelrail/internal/agent/pinned"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Errors returned to API callers.
var (
	ErrNotActive = errors.New("migrateagent: session is revoked or expired")
	ErrOffline   = errors.New("migrateagent: the source agent is not connected")
)

// Store is the persistence surface Service needs; *store.DB satisfies it.
type Store interface {
	SaveMigrationAgentSession(ctx context.Context, s store.MigrationAgentSession) error
	GetMigrationAgentSession(ctx context.Context, id string) (store.MigrationAgentSession, error)
	ListMigrationAgentSessions(ctx context.Context) ([]store.MigrationAgentSession, error)
	RedeemMigrationAgentToken(ctx context.Context, id, fingerprint, serial string, sessionExpires, now time.Time) error
	SetMigrationAgentState(ctx context.Context, id, state string, now time.Time) error
	TouchMigrationAgentSession(ctx context.Context, id, version, goos, arch string, now time.Time) error
	SaveMigrationAgentInventory(ctx context.Context, id, inventoryJSON string, now time.Time) error
	SaveMigrationAgentScan(ctx context.Context, id, scanJSON string, now time.Time) error
	AuditSaver
}

// Signer issues the short-lived client certificate; *agent.CA satisfies it.
type Signer interface {
	SignClientCSR(commonName string, csrDER []byte, validFor time.Duration, now time.Time) (agent.IssuedCert, error)
	CertPEM() []byte
}

// Service implements migrationpb.MigrationServiceServer and the live-session hub.
type Service struct {
	migrationpb.UnimplementedMigrationServiceServer
	st          Store
	ca          Signer
	cfg         Config
	log         *slog.Logger
	audit       *Auditor
	now         func() time.Time
	enrollLimit *ipLimiter

	mu   sync.Mutex
	live map[string]*liveConn
}

// NewService builds the service. now may be nil for the wall clock.
func NewService(st Store, ca Signer, cfg Config, logger *slog.Logger, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	cfg = cfg.withDefaults()
	return &Service{st: st, ca: ca, cfg: cfg, log: logger, now: now, live: map[string]*liveConn{},
		enrollLimit: newIPLimiter(cfg.EnrollRate, cfg.EnrollWindow), audit: &Auditor{Saver: st, Logger: logger, Now: now}}
}

// Auditor exposes the audit writer so API handlers record their events the same way.
func (s *Service) Auditor() *Auditor { return s.audit }

// Config returns the effective limits.
func (s *Service) Config() Config { return s.cfg }

// Created is a new pending session. Token is shown once and never stored.
type Created struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Create issues a single-use token bound to a new session.
func (s *Service) Create(ctx context.Context, label, actor string) (Created, error) {
	id, token, hash, err := NewToken()
	if err != nil {
		return Created{}, err
	}
	now := s.now()
	exp := now.Add(s.cfg.TokenTTL)
	err = s.st.SaveMigrationAgentSession(ctx, store.MigrationAgentSession{ID: id, Label: label, TokenHash: hash,
		TokenExpiresAt: exp, CreatedBy: actor, CreatedAt: now, UpdatedAt: now})
	if err != nil {
		return Created{}, fmt.Errorf("migrateagent: create session: %w", err)
	}
	s.audit.Record(ctx, Event{Action: ActionTokenIssued, SessionID: id, Actor: actor,
		Detail: map[string]any{"label": label, "token_expires_at": exp.UTC().Format(time.RFC3339)}})
	return Created{ID: id, Token: token, ExpiresAt: exp}, nil
}

// SessionView is the API shape of a session. It never carries token material.
type SessionView struct {
	ID               string     `json:"id"`
	Label            string     `json:"label"`
	State            string     `json:"state"`
	TokenExpiresAt   time.Time  `json:"token_expires_at"`
	SessionExpiresAt *time.Time `json:"session_expires_at,omitempty"`
	ConnectedAt      *time.Time `json:"connected_at,omitempty"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
	AgentVersion     string     `json:"agent_version,omitempty"`
	AgentOS          string     `json:"agent_os,omitempty"`
	AgentArch        string     `json:"agent_arch,omitempty"`
	CreatedBy        string     `json:"created_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
}

// EffectiveState folds time and liveness into the stored state: an unused token
// past its expiry or a session past its cap reads as expired, and a connected
// session whose stream is gone reads as lost.
func EffectiveState(sess store.MigrationAgentSession, live bool, now time.Time) string {
	switch sess.State {
	case store.MigrationAgentRevoked:
		return store.MigrationAgentRevoked
	case store.MigrationAgentPending:
		if now.After(sess.TokenExpiresAt) {
			return store.MigrationAgentExpired
		}
	case store.MigrationAgentConnected, store.MigrationAgentLost:
		if sess.SessionExpiresAt != nil && now.After(*sess.SessionExpiresAt) {
			return store.MigrationAgentExpired
		}
		if live {
			return store.MigrationAgentConnected
		}
		return store.MigrationAgentLost
	}
	return sess.State
}

func (s *Service) view(sess store.MigrationAgentSession) SessionView {
	s.mu.Lock()
	_, live := s.live[sess.ID]
	s.mu.Unlock()
	return SessionView{ID: sess.ID, Label: sess.Label, State: EffectiveState(sess, live, s.now()), TokenExpiresAt: sess.TokenExpiresAt,
		SessionExpiresAt: sess.SessionExpiresAt, ConnectedAt: sess.ConnectedAt, LastSeenAt: sess.LastSeenAt, AgentVersion: sess.AgentVersion,
		AgentOS: sess.AgentOS, AgentArch: sess.AgentArch, CreatedBy: sess.CreatedBy, CreatedAt: sess.CreatedAt}
}

// List returns every session, newest first.
func (s *Service) List(ctx context.Context) ([]SessionView, error) {
	rows, err := s.st.ListMigrationAgentSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("migrateagent: list sessions: %w", err)
	}
	out := make([]SessionView, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.view(r))
	}
	return out, nil
}

// Get returns one session with its stored inventory and scan.
func (s *Service) Get(ctx context.Context, id string) (SessionView, InventoryDoc, ScanDoc, error) {
	row, err := s.st.GetMigrationAgentSession(ctx, id)
	if err != nil {
		return SessionView{}, InventoryDoc{}, ScanDoc{}, fmt.Errorf("migrateagent: get session: %w", err)
	}
	return s.view(row), ParseInventory(row.InventoryJSON), ParseScan(row.ScanJSON), nil
}

// Revoke ends a session for good: the state is persisted first so the revoke holds
// even when the source host is unreachable, then any live stream is closed.
func (s *Service) Revoke(ctx context.Context, id, actor string) error {
	if err := s.st.SetMigrationAgentState(ctx, id, store.MigrationAgentRevoked, s.now()); err != nil {
		return fmt.Errorf("migrateagent: revoke: %w", err)
	}
	s.mu.Lock()
	c := s.live[id]
	s.mu.Unlock()
	if c != nil {
		c.kickWith("revoked")
	}
	s.audit.Record(ctx, Event{Action: ActionRevoked, SessionID: id, Actor: actor})
	return nil
}

// refuseEnroll audits only refusals for a session that exists, so a caller with
// no valid id cannot write database rows or choose what the log says.
func (s *Service) refuseEnroll(ctx context.Context, id, reason string) error {
	if id == "" {
		s.log.Debug("migrateagent: enroll refused", slog.String("reason", reason))
		return status.Error(codes.Unauthenticated, "invalid migration token")
	}
	s.audit.Record(ctx, Event{Action: ActionEnrollRefused, SessionID: id, Actor: "migration-agent", Status: 401,
		Remote: remoteAddr(ctx), Detail: map[string]any{"reason": reason}})
	return status.Error(codes.Unauthenticated, "invalid migration token")
}

// Enroll redeems a single-use token for a short-lived client certificate.
func (s *Service) Enroll(ctx context.Context, req *migrationpb.EnrollRequest) (*migrationpb.EnrollResponse, error) {
	if !s.enrollLimit.allow(hostOnly(remoteAddr(ctx)), s.now()) {
		return nil, status.Error(codes.ResourceExhausted, "too many enroll attempts")
	}
	id, secret, ok := ParseToken(req.GetJoinToken())
	if !ok {
		return nil, s.refuseEnroll(ctx, "", "malformed token")
	}
	if lim := s.cfg.AgentFieldMax; len(req.GetVersion()) > lim || len(req.GetOs()) > lim || len(req.GetArch()) > lim {
		return nil, status.Error(codes.InvalidArgument, "agent version, os or arch is too long")
	}
	sess, err := s.st.GetMigrationAgentSession(ctx, id)
	stored := sess.TokenHash
	if err != nil {
		stored = HashSecret("no such session")
	}
	match := subtle.ConstantTimeCompare([]byte(stored), []byte(HashSecret(secret))) == 1
	switch {
	case err != nil && !errors.Is(err, store.ErrMigrationAgentNotFound):
		s.log.Error("migrateagent: enroll lookup failed", slog.String("session_id", id), slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	case err != nil:
		return nil, s.refuseEnroll(ctx, "", "unknown session")
	case !match:
		return nil, s.refuseEnroll(ctx, id, "wrong secret")
	case sess.State != store.MigrationAgentPending || sess.TokenUsedAt != nil:
		return nil, s.refuseEnroll(ctx, id, "token already used or session "+sess.State)
	case !s.now().Before(sess.TokenExpiresAt):
		return nil, s.refuseEnroll(ctx, id, "token expired")
	case len(req.GetCsrDer()) == 0:
		return nil, status.Error(codes.InvalidArgument, "csr_der is required")
	}
	now := s.now()
	issued, err := s.ca.SignClientCSR(CertCommonNamePrefix+id, req.GetCsrDer(), s.cfg.SessionMax, now)
	if errors.Is(err, agent.ErrInvalidCSR) {
		return nil, status.Error(codes.InvalidArgument, "invalid certificate request")
	}
	if err != nil {
		s.log.Error("migrateagent: sign csr failed", slog.String("session_id", id), slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	sessionExpires := now.Add(s.cfg.SessionMax)
	if err := s.st.RedeemMigrationAgentToken(ctx, id, issued.Fingerprint, issued.Serial, sessionExpires, now); err != nil {
		if errors.Is(err, store.ErrMigrationAgentTokenUsed) {
			return nil, s.refuseEnroll(ctx, id, "token already used or expired")
		}
		s.log.Error("migrateagent: redeem failed", slog.String("session_id", id), slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "internal error")
	}
	_ = s.st.TouchMigrationAgentSession(ctx, id, req.GetVersion(), req.GetOs(), req.GetArch(), now)
	s.audit.Record(ctx, Event{Action: ActionEnrolled, SessionID: id, Actor: "migration-agent", Remote: remoteAddr(ctx),
		Detail: map[string]any{"ca_pin_state": req.GetCaPinState(), "agent_version": req.GetVersion(), "os": req.GetOs(), "arch": req.GetArch(),
			"cert_fingerprint": issued.Fingerprint, "session_expires_at": sessionExpires.UTC().Format(time.RFC3339)}})
	return &migrationpb.EnrollResponse{SessionId: id, ClientCertPem: issued.PEM, CaCertPem: s.ca.CertPEM(),
		NotAfter: timestamppb.New(issued.NotAfter), SessionExpiresAt: timestamppb.New(sessionExpires)}, nil
}

func remoteAddr(ctx context.Context) string {
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		return p.Addr.String()
	}
	return ""
}

// authenticate maps the verified client certificate to an active session.
func (s *Service) authenticate(ctx context.Context) (store.MigrationAgentSession, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return store.MigrationAgentSession{}, status.Error(codes.Unauthenticated, "no peer")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok || len(tlsInfo.State.PeerCertificates) == 0 {
		return store.MigrationAgentSession{}, status.Error(codes.Unauthenticated, "client certificate required")
	}
	leaf := tlsInfo.State.PeerCertificates[0]
	return s.sessionForCert(ctx, leaf)
}

func (s *Service) sessionForCert(ctx context.Context, leaf *x509.Certificate) (store.MigrationAgentSession, error) {
	id, ok := strings.CutPrefix(leaf.Subject.CommonName, CertCommonNamePrefix)
	if !ok {
		return store.MigrationAgentSession{}, status.Error(codes.PermissionDenied, "not a migration certificate")
	}
	deny := func(reason string) (store.MigrationAgentSession, error) {
		s.audit.Record(ctx, Event{Action: ActionSessionRejected, SessionID: id, Actor: "migration-agent", Status: 403,
			Remote: remoteAddr(ctx), Detail: map[string]any{"reason": reason}})
		return store.MigrationAgentSession{}, status.Error(codes.PermissionDenied, "migration session is not active")
	}
	sess, err := s.st.GetMigrationAgentSession(ctx, id)
	if err != nil {
		return deny("unknown session")
	}
	fp := pinned.Fingerprint(leaf.Raw)
	switch {
	case sess.CertFingerprint == "" || subtle.ConstantTimeCompare([]byte(sess.CertFingerprint), []byte(fp)) != 1:
		return deny("certificate does not match the session")
	case sess.State == store.MigrationAgentRevoked:
		return deny("session revoked")
	case sess.State != store.MigrationAgentConnected && sess.State != store.MigrationAgentLost:
		return deny("session " + sess.State)
	case sess.SessionExpiresAt == nil || !s.now().Before(*sess.SessionExpiresAt):
		_ = s.st.SetMigrationAgentState(ctx, id, store.MigrationAgentExpired, s.now())
		return deny("session expired")
	}
	return sess, nil
}

type liveConn struct {
	out     chan *migrationpb.ControlMessage
	kick    chan string
	dead    chan struct{}
	once    sync.Once
	mu      sync.Mutex
	pending map[string]chan *migrationpb.AgentMessage
}

func newLiveConn() *liveConn {
	return &liveConn{out: make(chan *migrationpb.ControlMessage, 8), kick: make(chan string, 1), dead: make(chan struct{}),
		pending: map[string]chan *migrationpb.AgentMessage{}}
}

func (c *liveConn) kickWith(reason string) {
	select {
	case c.kick <- reason:
	default:
	}
}

func (c *liveConn) deliver(reqID string, m *migrationpb.AgentMessage) {
	c.mu.Lock()
	ch := c.pending[reqID]
	c.mu.Unlock()
	if ch != nil {
		select {
		case ch <- m:
		default:
		}
	}
}

func randomRequestID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("migrateagent: request id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// request sends one control message and waits for the agent's reply.
func (s *Service) request(ctx context.Context, id string, build func(reqID string) *migrationpb.ControlMessage) (*migrationpb.AgentMessage, error) {
	sess, err := s.st.GetMigrationAgentSession(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("migrateagent: request: %w", err)
	}
	if st := EffectiveState(sess, true, s.now()); st == store.MigrationAgentRevoked || st == store.MigrationAgentExpired {
		return nil, ErrNotActive
	}
	s.mu.Lock()
	c := s.live[id]
	s.mu.Unlock()
	if c == nil {
		return nil, ErrOffline
	}
	reqID, err := randomRequestID()
	if err != nil {
		return nil, err
	}
	ch := make(chan *migrationpb.AgentMessage, 1)
	c.mu.Lock()
	c.pending[reqID] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, reqID)
		c.mu.Unlock()
	}()
	rctx, cancel := context.WithTimeout(ctx, s.cfg.RequestTimeout)
	defer cancel()
	select {
	case c.out <- build(reqID):
	case <-c.dead:
		return nil, ErrOffline
	case <-rctx.Done():
		return nil, fmt.Errorf("migrateagent: send request: %w", rctx.Err())
	}
	select {
	case m := <-ch:
		if d := m.GetDenied(); d != nil {
			return nil, fmt.Errorf("migrateagent: the agent refused: %s", d.GetReason())
		}
		return m, nil
	case <-c.dead:
		return nil, ErrOffline
	case <-rctx.Done():
		return nil, fmt.Errorf("migrateagent: wait for the agent: %w", rctx.Err())
	}
}

// Discover asks the agent for a fresh inventory and stores it.
func (s *Service) Discover(ctx context.Context, id string) (InventoryDoc, error) {
	m, err := s.request(ctx, id, func(reqID string) *migrationpb.ControlMessage {
		return &migrationpb.ControlMessage{Payload: &migrationpb.ControlMessage_Discover{Discover: &migrationpb.DiscoverRequest{RequestId: reqID}}}
	})
	if err != nil {
		return InventoryDoc{}, err
	}
	doc := InventoryFromPB(m.GetInventory())
	raw, err := marshalJSON(doc)
	if err != nil {
		return InventoryDoc{}, err
	}
	if err := s.st.SaveMigrationAgentInventory(ctx, id, raw, s.now()); err != nil {
		return InventoryDoc{}, fmt.Errorf("migrateagent: save inventory: %w", err)
	}
	dbs := 0
	for _, c := range doc.Containers {
		if c.DatabaseEngine != "" {
			dbs++
		}
	}
	s.audit.Record(ctx, Event{Action: ActionDiscovery, SessionID: id, Actor: "migration-agent",
		Detail: map[string]any{"containers": len(doc.Containers), "volumes": len(doc.Volumes), "databases": dbs}})
	return doc, nil
}

// Scan asks the agent to count files and bytes for targets and stores the result.
func (s *Service) Scan(ctx context.Context, id string, targets []string) (ScanDoc, error) {
	m, err := s.request(ctx, id, func(reqID string) *migrationpb.ControlMessage {
		return &migrationpb.ControlMessage{Payload: &migrationpb.ControlMessage_Scan{Scan: &migrationpb.ScanRequest{RequestId: reqID, Targets: targets}}}
	})
	if err != nil {
		return ScanDoc{}, err
	}
	doc := ScanFromPB(m.GetScanResult(), s.now())
	raw, err := marshalJSON(doc)
	if err != nil {
		return ScanDoc{}, err
	}
	if err := s.st.SaveMigrationAgentScan(ctx, id, raw, s.now()); err != nil {
		return ScanDoc{}, fmt.Errorf("migrateagent: save scan: %w", err)
	}
	s.audit.Record(ctx, Event{Action: ActionScan, SessionID: id, Actor: "migration-agent", Detail: map[string]any{"targets": len(targets)}})
	return doc, nil
}
