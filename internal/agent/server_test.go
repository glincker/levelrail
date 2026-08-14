package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeEnrollStore is a hand-written fake for EnrollStore, the same
// "not a mocking framework" convention every other package's tests in
// this codebase already follow.
type fakeEnrollStore struct {
	tokens map[string]*store.NodeJoinToken // keyed by hash
	nodes  map[string]*store.Node          // keyed by ID
	names  map[string]bool                 // taken names, for ErrNodeNameTaken

	markUsedErr error
	saveNodeErr error
	getNodeErr  error

	updateStatusMu []store.NodeStatus // records every UpdateNodeStatus call, in order

	// touchMu guards lastTouchedID and touchCount: unlike every other
	// field here, TouchNodeLastSeen is called both synchronously
	// (Session's own initial touch) and repeatedly from
	// Server.heartbeatLoop's own background goroutine (TASKS.md 3.7)
	// for as long as a test's fake session stays open, so concurrent
	// access is real here, not theoretical.
	touchMu       sync.Mutex
	lastTouchedID string
	touchCount    int
}

func newFakeEnrollStore() *fakeEnrollStore {
	return &fakeEnrollStore{
		tokens: map[string]*store.NodeJoinToken{},
		nodes:  map[string]*store.Node{},
		names:  map[string]bool{},
	}
}

func (f *fakeEnrollStore) GetNodeJoinTokenByHash(_ context.Context, hash string) (*store.NodeJoinToken, error) {
	t, ok := f.tokens[hash]
	if !ok {
		return nil, store.ErrNodeJoinTokenNotFound
	}
	cp := *t
	return &cp, nil
}

func (f *fakeEnrollStore) MarkNodeJoinTokenUsed(_ context.Context, id string) error {
	if f.markUsedErr != nil {
		return f.markUsedErr
	}
	for _, t := range f.tokens {
		if t.ID == id {
			if t.UsedAt != nil {
				return store.ErrNodeJoinTokenAlreadyUsed
			}
			now := time.Now()
			t.UsedAt = &now
			return nil
		}
	}
	return store.ErrNodeJoinTokenNotFound
}

func (f *fakeEnrollStore) SaveNode(_ context.Context, n store.Node) error {
	if f.saveNodeErr != nil {
		return f.saveNodeErr
	}
	if f.names[n.Name] {
		return store.ErrNodeNameTaken
	}
	f.names[n.Name] = true
	cp := n
	f.nodes[n.ID] = &cp
	return nil
}

func (f *fakeEnrollStore) GetNode(_ context.Context, id string) (*store.Node, error) {
	if f.getNodeErr != nil {
		return nil, f.getNodeErr
	}
	n, ok := f.nodes[id]
	if !ok {
		return nil, store.ErrNodeNotFound
	}
	cp := *n
	return &cp, nil
}

func (f *fakeEnrollStore) UpdateNodeStatus(_ context.Context, id string, status store.NodeStatus) error {
	f.updateStatusMu = append(f.updateStatusMu, status)
	if n, ok := f.nodes[id]; ok {
		n.Status = status
	}
	return nil
}

func (f *fakeEnrollStore) TouchNodeLastSeen(_ context.Context, id string) error {
	f.touchMu.Lock()
	defer f.touchMu.Unlock()
	f.lastTouchedID = id
	f.touchCount++
	return nil
}

func (f *fakeEnrollStore) touchStats() (id string, count int) {
	f.touchMu.Lock()
	defer f.touchMu.Unlock()
	return f.lastTouchedID, f.touchCount
}

func seedJoinToken(f *fakeEnrollStore, plaintext string, expiresIn time.Duration) {
	f.tokens[hashJoinToken(plaintext)] = &store.NodeJoinToken{
		ID: "njt_" + plaintext, TokenHash: hashJoinToken(plaintext),
		CreatedAt: time.Now(), ExpiresAt: time.Now().Add(expiresIn),
	}
}

func TestServer_Enroll_Success(t *testing.T) {
	ca, err := GenerateCA()
	if err != nil {
		t.Fatalf("GenerateCA() error = %v", err)
	}
	st := newFakeEnrollStore()
	seedJoinToken(st, "plaintext-token", time.Hour)
	srv := NewServer(ca, st, NewRegistry(), nil)

	resp, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{JoinToken: "plaintext-token", NodeName: "worker-1"})
	if err != nil {
		t.Fatalf("Enroll() error = %v", err)
	}
	if resp.GetNodeId() == "" {
		t.Error("NodeId is empty")
	}
	if len(resp.GetClientCertPem()) == 0 || len(resp.GetClientKeyPem()) == 0 || len(resp.GetCaCertPem()) == 0 {
		t.Errorf("resp = %+v, want all three PEM fields populated", resp)
	}
	verifyChain(t, resp.GetCaCertPem(), resp.GetClientCertPem())

	node, err := st.GetNode(context.Background(), resp.GetNodeId())
	if err != nil {
		t.Fatalf("GetNode() error = %v", err)
	}
	if node.Name != "worker-1" || node.Status != store.NodeStatusPending {
		t.Errorf("saved node = %+v, want Name=worker-1 Status=pending", node)
	}
	if node.CertFingerprint == "" {
		t.Error("saved node has no CertFingerprint")
	}

	// The token must be consumed, not reusable.
	tok := st.tokens[hashJoinToken("plaintext-token")]
	if tok.UsedAt == nil {
		t.Error("join token UsedAt not set after a successful Enroll")
	}
}

func TestServer_Enroll_InvalidToken(t *testing.T) {
	ca, _ := GenerateCA()
	srv := NewServer(ca, newFakeEnrollStore(), NewRegistry(), nil)

	_, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{JoinToken: "nonexistent", NodeName: "worker-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("Enroll() error = %v, want codes.Unauthenticated", err)
	}
}

func TestServer_Enroll_ExpiredToken(t *testing.T) {
	ca, _ := GenerateCA()
	st := newFakeEnrollStore()
	seedJoinToken(st, "plaintext-token", -time.Hour) // already expired
	srv := NewServer(ca, st, NewRegistry(), nil)

	_, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{JoinToken: "plaintext-token", NodeName: "worker-1"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("Enroll() error = %v, want codes.Unauthenticated", err)
	}
}

func TestServer_Enroll_AlreadyUsedToken(t *testing.T) {
	ca, _ := GenerateCA()
	st := newFakeEnrollStore()
	seedJoinToken(st, "plaintext-token", time.Hour)
	srv := NewServer(ca, st, NewRegistry(), nil)

	if _, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{JoinToken: "plaintext-token", NodeName: "worker-1"}); err != nil {
		t.Fatalf("first Enroll() error = %v", err)
	}
	_, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{JoinToken: "plaintext-token", NodeName: "worker-2"})
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("second Enroll() (reused token) error = %v, want codes.Unauthenticated", err)
	}
}

func TestServer_Enroll_DuplicateNodeName(t *testing.T) {
	ca, _ := GenerateCA()
	st := newFakeEnrollStore()
	seedJoinToken(st, "token-1", time.Hour)
	seedJoinToken(st, "token-2", time.Hour)
	srv := NewServer(ca, st, NewRegistry(), nil)

	if _, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{JoinToken: "token-1", NodeName: "worker-1"}); err != nil {
		t.Fatalf("first Enroll() error = %v", err)
	}
	_, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{JoinToken: "token-2", NodeName: "worker-1"})
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("Enroll() with a taken name error = %v, want codes.AlreadyExists", err)
	}
}

func TestServer_Enroll_MissingFields(t *testing.T) {
	ca, _ := GenerateCA()
	srv := NewServer(ca, newFakeEnrollStore(), NewRegistry(), nil)

	if _, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{NodeName: "worker-1"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("missing join_token: error = %v, want codes.InvalidArgument", err)
	}
	if _, err := srv.Enroll(context.Background(), &agentpb.EnrollRequest{JoinToken: "x"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("missing node_name: error = %v, want codes.InvalidArgument", err)
	}
}

// fakeAgentSessionServer is a hand-written fake for the full
// agentpb.AgentService_SessionServer interface (sessionStream's two
// methods plus grpc.ServerStream's), so Session can be tested without a
// real network connection.
type fakeAgentSessionServer struct {
	ctx     context.Context
	recv    chan *agentpb.AgentMessage
	sent    chan *agentpb.ControlMessage
	recvErr error
}

func (f *fakeAgentSessionServer) Send(msg *agentpb.ControlMessage) error {
	f.sent <- msg
	return nil
}
func (f *fakeAgentSessionServer) Recv() (*agentpb.AgentMessage, error) {
	msg, ok := <-f.recv
	if !ok {
		return nil, f.recvErr
	}
	return msg, nil
}
func (f *fakeAgentSessionServer) Context() context.Context     { return f.ctx }
func (f *fakeAgentSessionServer) SetHeader(metadata.MD) error  { return nil }
func (f *fakeAgentSessionServer) SendHeader(metadata.MD) error { return nil }
func (f *fakeAgentSessionServer) SetTrailer(metadata.MD)       {}
func (f *fakeAgentSessionServer) SendMsg(any) error            { return nil }
func (f *fakeAgentSessionServer) RecvMsg(any) error            { return nil }

// ctxWithPeerCert builds a context carrying gRPC peer info for a real
// leaf certificate, the same shape a real mTLS-authenticated
// connection's stream.Context() carries, so Session's peerIdentity
// extraction is exercised for real rather than mocked at that boundary.
func ctxWithPeerCert(certPEM []byte) context.Context {
	block, _ := pem.Decode(certPEM)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		panic(err)
	}
	return peer.NewContext(context.Background(), &peer.Peer{
		AuthInfo: credentials.TLSInfo{State: tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}},
	})
}

func TestServer_Session_Success(t *testing.T) {
	ca, _ := GenerateCA()
	st := newFakeEnrollStore()
	certPEM, _, err := ca.IssueClientCert("node-1", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() error = %v", err)
	}
	fp, err := certFingerprintFromPEM(certPEM)
	if err != nil {
		t.Fatalf("certFingerprintFromPEM() error = %v", err)
	}
	now := time.Now()
	st.nodes["node-1"] = &store.Node{ID: "node-1", Name: "worker-1", CertFingerprint: fp, CreatedAt: now, UpdatedAt: now}

	registry := NewRegistry()
	srv := NewServer(ca, st, registry, nil)

	stream := &fakeAgentSessionServer{
		ctx:  ctxWithPeerCert(certPEM),
		recv: make(chan *agentpb.AgentMessage),
		sent: make(chan *agentpb.ControlMessage, 4),
	}

	sessionDone := make(chan error, 1)
	go func() { sessionDone <- srv.Session(stream) }()

	// Give Session a moment to register the transport before checking.
	deadline := time.After(2 * time.Second)
	for {
		if _, err := registry.Get("node-1"); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for the transport to be registered")
		case <-time.After(10 * time.Millisecond):
		}
	}

	if lastID, _ := st.touchStats(); lastID != "node-1" {
		t.Errorf("lastTouchedID = %q, want node-1", lastID)
	}
	if len(st.updateStatusMu) == 0 || st.updateStatusMu[0] != store.NodeStatusOnline {
		t.Errorf("updateStatusMu = %v, want first entry Online", st.updateStatusMu)
	}

	// Ending the stream must unregister the transport and mark the node
	// offline.
	stream.recvErr = errors.New("connection reset")
	close(stream.recv)

	select {
	case err := <-sessionDone:
		if err != nil {
			t.Errorf("Session() error = %v, want nil (a stream ending is not itself a Session failure)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Session() to return")
	}

	if _, err := registry.Get("node-1"); err == nil {
		t.Error("transport still registered after the session ended")
	}
	if st.updateStatusMu[len(st.updateStatusMu)-1] != store.NodeStatusOffline {
		t.Errorf("final status = %v, want Offline", st.updateStatusMu[len(st.updateStatusMu)-1])
	}
}

// TestServer_Session_PeriodicHeartbeat is TASKS.md 3.7's real point:
// TouchNodeLastSeen must keep being called on an interval for as long as
// the session's stream stays open, not just once at connect. Without
// this, internal/reconcile/nodehealth would see LastSeenAt go stale on
// every long-lived, perfectly healthy connection and incorrectly flip it
// Offline.
func TestServer_Session_PeriodicHeartbeat(t *testing.T) {
	ca, _ := GenerateCA()
	st := newFakeEnrollStore()
	certPEM, _, err := ca.IssueClientCert("node-1", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() error = %v", err)
	}
	fp, err := certFingerprintFromPEM(certPEM)
	if err != nil {
		t.Fatalf("certFingerprintFromPEM() error = %v", err)
	}
	now := time.Now()
	st.nodes["node-1"] = &store.Node{ID: "node-1", Name: "worker-1", CertFingerprint: fp, CreatedAt: now, UpdatedAt: now}

	registry := NewRegistry()
	srv := NewServer(ca, st, registry, nil, WithHeartbeatInterval(10*time.Millisecond))

	stream := &fakeAgentSessionServer{
		ctx:  ctxWithPeerCert(certPEM),
		recv: make(chan *agentpb.AgentMessage),
		sent: make(chan *agentpb.ControlMessage, 4),
	}

	sessionDone := make(chan error, 1)
	go func() { sessionDone <- srv.Session(stream) }()

	// Wait for enough ticks that the loop must have fired more than
	// once: the connect-time touch (1) plus at least a couple of
	// interval ticks.
	deadline := time.After(2 * time.Second)
	for {
		if _, count := st.touchStats(); count >= 3 {
			break
		}
		select {
		case <-deadline:
			_, count := st.touchStats()
			t.Fatalf("timed out waiting for periodic heartbeat touches, got count=%d, want >= 3", count)
		case <-time.After(5 * time.Millisecond):
		}
	}

	stream.recvErr = errors.New("connection reset")
	close(stream.recv)

	select {
	case <-sessionDone:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Session() to return")
	}

	// The heartbeat loop must actually stop once the session ends, not
	// keep touching a node that's no longer connected.
	_, countAtEnd := st.touchStats()
	time.Sleep(50 * time.Millisecond)
	_, countAfterWait := st.touchStats()
	if countAfterWait != countAtEnd {
		t.Errorf("touch count kept growing after Session() returned (%d -> %d), want the heartbeat loop to have stopped", countAtEnd, countAfterWait)
	}
}

func TestServer_Session_NoPeerInfo_Rejected(t *testing.T) {
	ca, _ := GenerateCA()
	srv := NewServer(ca, newFakeEnrollStore(), NewRegistry(), nil)

	stream := &fakeAgentSessionServer{ctx: context.Background(), recv: make(chan *agentpb.AgentMessage), sent: make(chan *agentpb.ControlMessage, 1)}
	err := srv.Session(stream)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("Session() error = %v, want codes.Unauthenticated", err)
	}
}

func TestServer_Session_UnknownNode_Rejected(t *testing.T) {
	ca, _ := GenerateCA()
	certPEM, _, err := ca.IssueClientCert("node-ghost", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() error = %v", err)
	}
	srv := NewServer(ca, newFakeEnrollStore(), NewRegistry(), nil) // no node saved

	stream := &fakeAgentSessionServer{ctx: ctxWithPeerCert(certPEM), recv: make(chan *agentpb.AgentMessage), sent: make(chan *agentpb.ControlMessage, 1)}
	err = srv.Session(stream)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("Session() error = %v, want codes.Unauthenticated", err)
	}
}

func TestServer_Session_FingerprintMismatch_Rejected(t *testing.T) {
	ca, _ := GenerateCA()
	st := newFakeEnrollStore()
	certPEM, _, err := ca.IssueClientCert("node-1", time.Hour)
	if err != nil {
		t.Fatalf("IssueClientCert() error = %v", err)
	}
	now := time.Now()
	// The stored fingerprint doesn't match this certificate at all: a
	// different certificate must have been issued for this node ID at
	// some point, or the record was tampered with.
	st.nodes["node-1"] = &store.Node{ID: "node-1", Name: "worker-1", CertFingerprint: "deadbeef", CreatedAt: now, UpdatedAt: now}
	srv := NewServer(ca, st, NewRegistry(), nil)

	stream := &fakeAgentSessionServer{ctx: ctxWithPeerCert(certPEM), recv: make(chan *agentpb.AgentMessage), sent: make(chan *agentpb.ControlMessage, 1)}
	err = srv.Session(stream)
	if status.Code(err) != codes.Unauthenticated {
		t.Errorf("Session() error = %v, want codes.Unauthenticated", err)
	}
}
