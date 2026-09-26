package agent

// Certificate lifecycle over the real gRPC transport: a real store, an
// in-process CA, a real TLS listener, and the agent-side client code. No
// Docker needed.

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type certTestServer struct {
	addr     string
	srv      *Server
	grpc     *grpc.Server
	dropNext atomic.Bool // commit the next Renew, then fail its response
}

func startCertTestServer(t *testing.T, ca *CA, st EnrollStore, opts ...Option) *certTestServer {
	t.Helper()
	creds, err := NewServerCredentials(ca, []string{"127.0.0.1"}, time.Hour)
	if err != nil {
		t.Fatalf("NewServerCredentials() error = %v", err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	ts := &certTestServer{addr: lis.Addr().String()}
	ts.srv = NewServer(ca, st, NewRegistry(), nil, opts...)
	ts.grpc = grpc.NewServer(grpc.Creds(creds), grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		resp, err := h(ctx, req)
		if err == nil && info.FullMethod == agentpb.AgentService_Renew_FullMethodName && ts.dropNext.CompareAndSwap(true, false) {
			return nil, status.Error(codes.Unavailable, "connection reset before the response arrived")
		}
		return resp, err
	}))
	agentpb.RegisterAgentServiceServer(ts.grpc, ts.srv)
	go func() { _ = ts.grpc.Serve(lis) }()
	t.Cleanup(ts.grpc.Stop)
	return ts
}

func mintToken(t *testing.T, db *store.DB, plaintext, purpose, nodeID string) {
	t.Helper()
	now := time.Now()
	if err := db.SaveNodeJoinToken(context.Background(), store.NodeJoinToken{
		ID: "njt_" + plaintext, TokenHash: hashJoinToken(plaintext), CreatedAt: now, ExpiresAt: now.Add(time.Hour),
		Purpose: purpose, NodeID: nodeID,
	}); err != nil {
		t.Fatalf("SaveNodeJoinToken() error = %v", err)
	}
}

func dialAs(t *testing.T, addr string, id *Identity) agentpb.AgentServiceClient {
	t.Helper()
	conn, err := dialWithIdentity(addr, id)
	if err != nil {
		t.Fatalf("dialWithIdentity() error = %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return agentpb.NewAgentServiceClient(conn)
}

func checkCurrent(t *testing.T, addr string, id *Identity) (current bool, err error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	resp, err := dialAs(t, addr, id).CheckIdentity(ctx, &agentpb.CheckIdentityRequest{})
	if err != nil {
		return false, err
	}
	return resp.GetCurrent(), nil
}

func enrollNode(t *testing.T, db *store.DB, ts *certTestServer, name string) *Identity {
	t.Helper()
	mintToken(t, db, "enroll-"+name, store.NodeJoinTokenPurposeEnroll, "")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	id, err := DialEnroll(ctx, ts.addr, "enroll-"+name, name)
	if err != nil {
		t.Fatalf("DialEnroll() error = %v", err)
	}
	return id
}

func TestCertLifecycle_EnrollWithCSR_KeyNeverLeavesAgent(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	id := enrollNode(t, db, ts, "csr-node")

	node, err := db.GetNode(context.Background(), id.NodeID)
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if node.CertKeyOrigin != store.CertKeyOriginAgent {
		t.Errorf("CertKeyOrigin = %q, want agent", node.CertKeyOrigin)
	}
	if node.CertNotAfter == nil || node.CertSerial == "" {
		t.Errorf("enrollment must record expiry and serial, got %+v", node)
	}
	if current, err := checkCurrent(t, ts.addr, id); err != nil || !current {
		t.Fatalf("CheckIdentity() = %v, %v; want current", current, err)
	}
}

func TestCertLifecycle_LegacyEnrollWithoutCSR(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ctx := context.Background()

	for _, tt := range []struct {
		name       string
		requireCSR bool
		wantCode   codes.Code
	}{
		{"accepted by default", false, codes.OK},
		{"refused when CSR is required", true, codes.FailedPrecondition},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ts := startCertTestServer(t, ca, db, WithRequireCSR(tt.requireCSR))
			token := "legacy-" + tt.wantCode.String()
			mintToken(t, db, token, store.NodeJoinTokenPurposeEnroll, "")
			conn, err := grpc.NewClient(ts.addr, grpc.WithTransportCredentials(credentials.NewTLS(enrollTrustConfig(ca.Fingerprint()))))
			if err != nil {
				t.Fatalf("grpc.NewClient() error = %v", err)
			}
			defer func() { _ = conn.Close() }()
			resp, err := agentpb.NewAgentServiceClient(conn).Enroll(ctx, &agentpb.EnrollRequest{JoinToken: token, NodeName: "old-agent-" + tt.wantCode.String()})
			if status.Code(err) != tt.wantCode {
				t.Fatalf("Enroll() code = %v (%v), want %v", status.Code(err), err, tt.wantCode)
			}
			if err == nil && len(resp.GetClientKeyPem()) == 0 {
				t.Fatal("an agent that sent no CSR needs the server-generated key back")
			}
		})
	}
}

func TestCertLifecycle_RenewOverlapAndExpiryOfPrevious(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	clock := &fakeClock{now: time.Now()}
	grace := 2 * time.Hour
	ts := startCertTestServer(t, ca, db, WithClock(clock.Now), WithRenewGrace(grace))
	old := enrollNode(t, db, ts, "renew-node")

	file := NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
	if err := file.Save(old); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	holder := NewIdentityHolder(old)
	r := NewRenewer(holder, file, CheckIdentityAt(ts.addr), RenewConfig{}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.RenewOnce(ctx, dialAs(t, ts.addr, old)); err != nil {
		t.Fatalf("RenewOnce() error = %v", err)
	}
	renewed := holder.Current()
	if renewed == old {
		t.Fatal("holder still has the old identity")
	}
	onDisk, err := file.Load()
	if err != nil || string(onDisk.ClientCertPEM) != string(renewed.ClientCertPEM) || file.HasStaged() {
		t.Fatalf("renewed identity not durably confirmed on disk: err=%v staged=%v", err, file.HasStaged())
	}

	if current, err := checkCurrent(t, ts.addr, old); err != nil || current {
		t.Fatalf("old cert inside the overlap: current=%v err=%v, want accepted as previous", current, err)
	}
	clock.Advance(grace + time.Minute)
	if _, err := checkCurrent(t, ts.addr, old); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("old cert after the overlap: err=%v, want Unauthenticated", err)
	}
	if current, err := checkCurrent(t, ts.addr, renewed); err != nil || !current {
		t.Fatalf("renewed cert: current=%v err=%v", current, err)
	}
	node, _ := db.GetNode(context.Background(), old.NodeID)
	if node.CertGeneration != 2 || node.CertRenewedAt == nil {
		t.Errorf("node after renewal: generation=%d renewed_at=%v", node.CertGeneration, node.CertRenewedAt)
	}
}

func TestCertLifecycle_RenewalResponseLost_ThenRetrySucceeds(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	old := enrollNode(t, db, ts, "lost-response-node")

	file := NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
	_ = file.Save(old)
	holder := NewIdentityHolder(old)
	r := NewRenewer(holder, file, CheckIdentityAt(ts.addr), RenewConfig{}, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ts.dropNext.Store(true)
	if err := r.RenewOnce(ctx, dialAs(t, ts.addr, old)); err == nil {
		t.Fatal("RenewOnce() succeeded although its response was dropped")
	}
	if holder.Current() != old || file.HasStaged() {
		t.Fatal("a lost response must leave the agent on its old identity with nothing staged")
	}
	// The server did commit: the old cert is now only the previous one,
	// and still works, so the node is not locked out.
	if current, err := checkCurrent(t, ts.addr, old); err != nil || current {
		t.Fatalf("old cert after lost response: current=%v err=%v", current, err)
	}
	if err := r.RenewOnce(ctx, dialAs(t, ts.addr, old)); err != nil {
		t.Fatalf("retry RenewOnce() error = %v", err)
	}
	if current, err := checkCurrent(t, ts.addr, holder.Current()); err != nil || !current {
		t.Fatalf("retried cert: current=%v err=%v", current, err)
	}
	node, _ := db.GetNode(context.Background(), old.NodeID)
	if node.CertGeneration != 3 || node.PrevCertFingerprint == "" {
		t.Errorf("after retry: generation=%d prev=%q", node.CertGeneration, node.PrevCertFingerprint)
	}
}

func TestCertLifecycle_ControlPlaneRestartsMidRenewal(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	first := startCertTestServer(t, ca, db)
	old := enrollNode(t, db, first, "restart-node")
	holder := NewIdentityHolder(old)
	file := NewIdentityFile(filepath.Join(t.TempDir(), "identity.json"))
	_ = file.Save(old)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first.dropNext.Store(true)
	_ = NewRenewer(holder, file, CheckIdentityAt(first.addr), RenewConfig{}, nil).RenewOnce(ctx, dialAs(t, first.addr, old))
	first.grpc.Stop()

	second := startCertTestServer(t, ca, db)
	r := NewRenewer(holder, file, CheckIdentityAt(second.addr), RenewConfig{}, nil)
	if err := r.RenewOnce(ctx, dialAs(t, second.addr, holder.Current())); err != nil {
		t.Fatalf("RenewOnce() after restart error = %v", err)
	}
	if current, err := checkCurrent(t, second.addr, holder.Current()); err != nil || !current {
		t.Fatalf("renewed after restart: current=%v err=%v", current, err)
	}
}

func TestCertLifecycle_ReenrollAfterRevoke(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	old := enrollNode(t, db, ts, "reenroll-node")
	other := enrollNode(t, db, ts, "other-node")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := db.RevokeNodeCert(ctx, old.NodeID, time.Now()); err != nil {
		t.Fatalf("RevokeNodeCert() error = %v", err)
	}
	if _, err := checkCurrent(t, ts.addr, old); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("revoked cert: err=%v, want PermissionDenied", err)
	}
	if _, err := dialAs(t, ts.addr, old).Renew(ctx, &agentpb.RenewRequest{CsrDer: mustCSR(t)}); status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Renew with revoked cert: err=%v, want PermissionDenied", err)
	}

	mintToken(t, db, "plain-enroll", store.NodeJoinTokenPurposeEnroll, "")
	if _, err := DialReenroll(ctx, ts.addr, "plain-enroll", old.NodeID, WithTrustedCA(old.CACertPEM)); status.Code(errors.Unwrap(err)) != codes.Unauthenticated {
		t.Fatalf("Reenroll with an enroll token: err=%v, want Unauthenticated", err)
	}
	mintToken(t, db, "for-other", store.NodeJoinTokenPurposeReenroll, other.NodeID)
	if _, err := DialReenroll(ctx, ts.addr, "for-other", old.NodeID, WithTrustedCA(old.CACertPEM)); status.Code(errors.Unwrap(err)) != codes.PermissionDenied {
		t.Fatalf("Reenroll with another node's token: err=%v, want PermissionDenied", err)
	}
	mintToken(t, db, "reenroll-it", store.NodeJoinTokenPurposeReenroll, old.NodeID)
	if _, err := DialEnroll(ctx, ts.addr, "reenroll-it", "sneaky-new-node"); status.Code(errors.Unwrap(err)) != codes.Unauthenticated {
		t.Fatalf("Enroll with a re-enroll token: err=%v, want Unauthenticated", err)
	}

	mintToken(t, db, "reenroll-ok", store.NodeJoinTokenPurposeReenroll, old.NodeID)
	fresh, err := DialReenroll(ctx, ts.addr, "reenroll-ok", "", WithTrustedCA(old.CACertPEM))
	if err != nil {
		t.Fatalf("DialReenroll() error = %v", err)
	}
	if fresh.NodeID != old.NodeID {
		t.Fatalf("re-enrolled as %q, want to keep %q", fresh.NodeID, old.NodeID)
	}
	if current, err := checkCurrent(t, ts.addr, fresh); err != nil || !current {
		t.Fatalf("re-enrolled cert: current=%v err=%v", current, err)
	}
	if _, err := checkCurrent(t, ts.addr, old); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("pre-revocation cert after re-enroll: err=%v, want Unauthenticated", err)
	}
	if _, err := DialReenroll(ctx, ts.addr, "reenroll-ok", "", WithTrustedCA(old.CACertPEM)); err == nil {
		t.Fatal("a re-enrollment token must be single use")
	}
}

func mustCSR(t *testing.T) []byte {
	t.Helper()
	_, csr, err := NewKeyAndCSR("x")
	if err != nil {
		t.Fatalf("NewKeyAndCSR() error = %v", err)
	}
	return csr
}

func TestCertLifecycle_SessionSendsHelloAndRevokeDisconnects(t *testing.T) {
	db := openLiveTestStore(t)
	ca, _ := GenerateCA()
	ts := startCertTestServer(t, ca, db)
	id := enrollNode(t, db, ts, "hello-node")

	sessionCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunSession(sessionCtx, ts.addr, id, nil, nil) }()

	deadline := time.Now().Add(10 * time.Second)
	for {
		n, err := db.GetNode(context.Background(), id.NodeID)
		if err == nil && n.AgentReportedAt != nil && n.Status == store.NodeStatusOnline {
			want := LocalAgentInfo()
			if n.AgentVersion != want.GetVersion() || n.AgentOS != want.GetOs() || n.AgentArch != want.GetArch() {
				t.Fatalf("stored agent info %q/%q/%q, want %v", n.AgentVersion, n.AgentOS, n.AgentArch, want)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the Hello frame to be recorded")
		}
		time.Sleep(20 * time.Millisecond)
	}

	if err := db.RevokeNodeCert(context.Background(), id.NodeID, time.Now()); err != nil {
		t.Fatalf("RevokeNodeCert() error = %v", err)
	}
	ts.srv.Disconnect(id.NodeID)
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("RunSession() returned nil after the control plane closed a revoked session")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("revoked session was not closed")
	}
}
