package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

// fakeRenewClient signs Renew requests with a real CA, or fails with err.
type fakeRenewClient struct {
	agentpb.AgentServiceClient
	ca       *CA
	nodeID   string
	validFor time.Duration
	err      error
	calls    int
}

func (f *fakeRenewClient) Renew(_ context.Context, req *agentpb.RenewRequest, _ ...grpc.CallOption) (*agentpb.RenewResponse, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	issued, err := f.ca.SignClientCSR(f.nodeID, req.GetCsrDer(), f.validFor, time.Now())
	if err != nil {
		return nil, err
	}
	return &agentpb.RenewResponse{ClientCertPem: issued.PEM, CaCertPem: f.ca.CertPEM(), NotAfter: timestamppb.New(issued.NotAfter)}, nil
}

type fakePersister struct {
	mu      sync.Mutex
	staged  *Identity
	events  []string
	failOn  string
	failErr error
}

func (p *fakePersister) record(ev string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.events = append(p.events, ev)
	if ev == p.failOn {
		return p.failErr
	}
	return nil
}

func (p *fakePersister) Stage(next *Identity) error {
	p.staged = next
	return p.record("stage")
}
func (p *fakePersister) Confirm() error  { return p.record("confirm") }
func (p *fakePersister) Rollback() error { return p.record("rollback") }

func issuedIdentity(t *testing.T, ca *CA, nodeID string, validFor time.Duration) *Identity {
	t.Helper()
	keyPEM, csr, err := NewKeyAndCSR(nodeID)
	if err != nil {
		t.Fatalf("NewKeyAndCSR() error = %v", err)
	}
	issued, err := ca.SignClientCSR(nodeID, csr, validFor, time.Now())
	if err != nil {
		t.Fatalf("SignClientCSR() error = %v", err)
	}
	return &Identity{NodeID: nodeID, ClientCertPEM: issued.PEM, ClientKeyPEM: keyPEM, CACertPEM: ca.CertPEM()}
}

func TestRenewer_RenewAt(t *testing.T) {
	ca, _ := GenerateCA()
	id := issuedIdentity(t, ca, "node-1", 90*24*time.Hour)
	leaf, _ := id.Leaf()
	life := leaf.NotAfter.Sub(leaf.NotBefore)

	tests := []struct {
		name   string
		cfg    RenewConfig
		jitter float64
		want   time.Duration
	}{
		{"default two thirds, no jitter draw", RenewConfig{}, 0, time.Duration(float64(life) * DefaultRenewFraction)},
		{"half", RenewConfig{Fraction: 0.5, Jitter: -1}, 0.9, life / 2},
		{"out of range fraction falls back", RenewConfig{Fraction: 1.5, Jitter: -1}, 0, time.Duration(float64(life) * DefaultRenewFraction)},
		{"jitter adds up to its fraction", RenewConfig{Fraction: 0.5, Jitter: 0.1}, 1, time.Duration(float64(life) * 0.6)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRenewer(NewIdentityHolder(id), &fakePersister{}, nil, tt.cfg, nil)
			r.jit = func() float64 { return tt.jitter }
			at, err := r.RenewAt(id)
			if err != nil {
				t.Fatalf("RenewAt() error = %v", err)
			}
			if got := at.Sub(leaf.NotBefore); (got - tt.want).Abs() > time.Second {
				t.Errorf("renew offset = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenewer_RenewOnce(t *testing.T) {
	ca, _ := GenerateCA()
	rejected := status.Error(codes.Unauthenticated, "nope")
	tests := []struct {
		name         string
		renewErr     error
		checkErr     error
		failOn       string
		wantErr      bool
		wantSwapped  bool
		wantEvents   []string
		wantAuthFail bool
	}{
		{"success stages, checks, swaps, confirms", nil, nil, "", false, true, []string{"stage", "confirm"}, false},
		{"renew rejected touches nothing", rejected, nil, "", true, false, nil, true},
		{"response lost touches nothing", status.Error(codes.Unavailable, "connection reset"), nil, "", true, false, nil, false},
		{"test connection fails rolls back", nil, errors.New("handshake failed"), "", true, false, []string{"stage", "rollback"}, false},
		{"persist fails keeps old", nil, nil, "stage", true, false, []string{"stage"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := issuedIdentity(t, ca, "node-1", time.Hour)
			holder := NewIdentityHolder(old)
			persist := &fakePersister{failOn: tt.failOn, failErr: errors.New("disk full")}
			var checked *Identity
			check := func(_ context.Context, id *Identity) error {
				checked = id
				return tt.checkErr
			}
			r := NewRenewer(holder, persist, check, RenewConfig{}, nil)
			client := &fakeRenewClient{ca: ca, nodeID: "node-1", validFor: 90 * 24 * time.Hour, err: tt.renewErr}

			err := r.RenewOnce(context.Background(), client)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RenewOnce() error = %v, wantErr %v", err, tt.wantErr)
			}
			if IsAuthRejection(err) != tt.wantAuthFail {
				t.Errorf("IsAuthRejection(%v) = %v, want %v", err, IsAuthRejection(err), tt.wantAuthFail)
			}
			swapped := holder.Current() != old
			if swapped != tt.wantSwapped {
				t.Fatalf("identity swapped = %v, want %v", swapped, tt.wantSwapped)
			}
			if tt.wantSwapped {
				if checked != holder.Current() {
					t.Error("the identity that passed the test connection is not the one now in use")
				}
				if string(holder.Current().ClientKeyPEM) == string(old.ClientKeyPEM) {
					t.Error("renewal must rotate the private key")
				}
			}
			if len(persist.events) != len(tt.wantEvents) {
				t.Fatalf("persist events = %v, want %v", persist.events, tt.wantEvents)
			}
			for i := range tt.wantEvents {
				if persist.events[i] != tt.wantEvents[i] {
					t.Fatalf("persist events = %v, want %v", persist.events, tt.wantEvents)
				}
			}
		})
	}
}

func TestRenewer_RunWaitsForThresholdThenRetriesWithBackoff(t *testing.T) {
	ca, _ := GenerateCA()
	id := issuedIdentity(t, ca, "node-1", 90*24*time.Hour)
	holder := NewIdentityHolder(id)
	r := NewRenewer(holder, &fakePersister{}, func(context.Context, *Identity) error { return nil },
		RenewConfig{Fraction: 0.5, Jitter: -1, RetryMin: time.Minute, RetryMax: 4 * time.Minute}, nil)

	leaf, _ := id.Leaf()
	var clockMu sync.Mutex
	now := leaf.NotBefore.Add(time.Hour)
	r.now = func() time.Time {
		clockMu.Lock()
		defer clockMu.Unlock()
		return now
	}
	waits := make(chan time.Duration, 16)
	r.after = func(d time.Duration) <-chan time.Time {
		waits <- d
		clockMu.Lock()
		now = now.Add(d)
		clockMu.Unlock()
		ch := make(chan time.Time, 1)
		ch <- r.now()
		return ch
	}
	client := &fakeRenewClient{ca: ca, nodeID: "node-1", validFor: 90 * 24 * time.Hour, err: status.Error(codes.Unavailable, "down")}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx, client); close(done) }()

	want := []time.Duration{leaf.NotAfter.Sub(leaf.NotBefore)/2 - time.Hour, time.Minute, 2 * time.Minute}
	for i, w := range want {
		got := <-waits
		if i == 0 && (got-w).Abs() > time.Second {
			t.Fatalf("first wait = %v, want about %v (until the renewal threshold)", got, w)
		}
		if i > 0 && got != w {
			t.Fatalf("retry wait %d = %v, want %v", i, got, w)
		}
	}
	for _, w := range []time.Duration{4 * time.Minute, 4 * time.Minute} {
		if got := <-waits; got != w {
			t.Fatalf("capped retry wait = %v, want %v", got, w)
		}
	}
	cancel()
	<-done
	if client.calls < 3 {
		t.Fatalf("Renew called %d times, want repeated retries", client.calls)
	}
}
