package agent

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/store"
)

// A second connection from the same node closes the first with Aborted, keeps the new transport registered and does not mark the node offline.
func TestServer_Session_DuplicateConnection_NewestWins(t *testing.T) {
	ca, _ := GenerateCA()
	st := newFakeEnrollStore()
	certPEM, _, err := issueClientCert(ca, "node-1")
	if err != nil {
		t.Fatalf("issueClientCert() error = %v", err)
	}
	fp, err := certFingerprintFromPEM(certPEM)
	if err != nil {
		t.Fatalf("certFingerprintFromPEM() error = %v", err)
	}
	now := time.Now()
	st.nodes["node-1"] = &store.Node{ID: "node-1", Name: "worker-1", CertFingerprint: fp, CreatedAt: now, UpdatedAt: now}

	registry := NewRegistry()
	srv := NewServer(ca, st, registry, nil)
	newStream := func() *fakeAgentSessionServer {
		return &fakeAgentSessionServer{ctx: ctxWithPeerCert(certPEM), recv: make(chan *agentpb.AgentMessage), sent: make(chan *agentpb.ControlMessage, 4)}
	}

	first, second := newStream(), newStream()
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	go func() { firstDone <- srv.Session(first) }()
	waitForTouchCount(t, st, 1)
	firstTransport, err := registry.Get("node-1")
	if err != nil {
		t.Fatalf("first transport not registered: %v", err)
	}

	go func() { secondDone <- srv.Session(second) }()

	select {
	case err := <-firstDone:
		if status.Code(err) != codes.Aborted {
			t.Fatalf("first Session() = %v, want Aborted", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("older session was not closed")
	}

	deadline := time.After(2 * time.Second)
	for {
		if cur, err := registry.Get("node-1"); err == nil && cur != firstTransport {
			break
		}
		select {
		case <-deadline:
			t.Fatal("the newer transport is not the registered one")
		case <-time.After(10 * time.Millisecond):
		}
	}
	time.Sleep(50 * time.Millisecond)
	if _, err := registry.Get("node-1"); err != nil {
		t.Fatalf("older session ending unregistered the newer one: %v", err)
	}
	for _, s := range st.updateStatusMu {
		if s == store.NodeStatusOffline {
			t.Fatal("node was marked offline while a newer session is live")
		}
	}

	second.recvErr = errors.New("connection reset")
	close(second.recv)
	select {
	case <-secondDone:
	case <-time.After(2 * time.Second):
		t.Fatal("second session did not end")
	}
	if _, err := registry.Get("node-1"); err == nil {
		t.Fatal("transport still registered after the current session ended")
	}
}
