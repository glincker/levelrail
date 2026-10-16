package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
)

func TestRenewer_ScheduleSurvivesRestartAndStaysInWindow(t *testing.T) {
	ca, _ := GenerateCA()
	id := issuedIdentity(t, ca, "node-1", 90*24*time.Hour)
	cfg := RenewConfig{Fraction: 0.6, Jitter: 0.1}
	first := NewRenewer(NewIdentityHolder(id), &fakePersister{}, nil, cfg, nil)
	restarted := NewRenewer(NewIdentityHolder(id), &fakePersister{}, nil, cfg, nil)

	a, err := first.RenewAt(id)
	if err != nil {
		t.Fatalf("RenewAt() error = %v", err)
	}
	b, _ := restarted.RenewAt(id)
	if !a.Equal(b) {
		t.Fatalf("a restarted agent re-rolled the schedule: %v vs %v", a, b)
	}
	leaf, _ := id.Leaf()
	life := leaf.NotAfter.Sub(leaf.NotBefore)
	if off := a.Sub(leaf.NotBefore); off < time.Duration(float64(life)*0.6) || off > time.Duration(float64(life)*0.7) {
		t.Fatalf("renew offset %v outside [60%%, 70%%] of %v", off, life)
	}
	other := issuedIdentity(t, ca, "node-2", 90*24*time.Hour)
	otherLeaf, _ := other.Leaf()
	if c, _ := first.RenewAt(other); c.Sub(otherLeaf.NotBefore) == a.Sub(leaf.NotBefore) {
		t.Error("two certificates got identical jitter: nodes enrolled together would renew together")
	}
}

func TestRenewer_StatusTracksFailureRetryAndRecovery(t *testing.T) {
	ca, _ := GenerateCA()
	id := issuedIdentity(t, ca, "node-1", 90*24*time.Hour)
	r := NewRenewer(NewIdentityHolder(id), &fakePersister{}, func(context.Context, *Identity) error { return nil },
		RenewConfig{Fraction: 0.5, Jitter: -1, RetryMin: time.Minute, RetryMax: 4 * time.Minute}, nil)
	leaf, _ := id.Leaf()
	var mu sync.Mutex
	now := leaf.NotBefore.Add(time.Hour)
	r.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	type wait struct {
		d  time.Duration
		st RenewalStatus
	}
	waits := make(chan wait, 16)
	r.after = func(d time.Duration) <-chan time.Time {
		select {
		case waits <- wait{d, r.Status()}:
		default:
		}
		mu.Lock()
		now = now.Add(d)
		mu.Unlock()
		ch := make(chan time.Time, 1)
		ch <- r.now()
		return ch
	}
	client := &flakyRenewClient{fakeRenewClient: fakeRenewClient{ca: ca, nodeID: "node-1", validFor: 90 * 24 * time.Hour}}
	client.down.Store(true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { r.Run(ctx, client); close(done) }()

	<-waits            // until the threshold
	<-waits            // first retry wait
	st := (<-waits).st // second retry wait: two failures behind it
	if st.ConsecutiveFailures < 2 || st.LastError == "" || st.LastFailedAt.IsZero() || st.NextRetryAt.IsZero() {
		t.Fatalf("status after failures = %+v, want failures, last error, failure time and next retry", st)
	}
	if st.NextRenewAt.IsZero() {
		t.Error("status must keep the scheduled renewal time")
	}

	client.down.Store(false)
	deadline := time.Now().Add(5 * time.Second)
	for r.Status().ConsecutiveFailures != 0 {
		select {
		case <-waits:
		default:
			time.Sleep(5 * time.Millisecond)
		}
		if time.Now().After(deadline) {
			t.Fatalf("renewal never recovered: %+v", r.Status())
		}
	}
	cancel()
	<-done
	if got := r.Status(); got.LastError != "" || !got.NextRetryAt.IsZero() {
		t.Fatalf("status after recovery = %+v, want failure state cleared", got)
	}
}

// flakyRenewClient fails Renew while down is set.
type flakyRenewClient struct {
	fakeRenewClient
	down atomic.Bool
}

func (f *flakyRenewClient) Renew(ctx context.Context, req *agentpb.RenewRequest, opts ...grpc.CallOption) (*agentpb.RenewResponse, error) {
	if f.down.Load() {
		return nil, status.Error(codes.Unavailable, "down")
	}
	return f.fakeRenewClient.Renew(ctx, req, opts...)
}

func TestRenewer_RenewNowSkipsTheWait(t *testing.T) {
	ca, _ := GenerateCA()
	id := issuedIdentity(t, ca, "node-1", 90*24*time.Hour)
	holder := NewIdentityHolder(id)
	r := NewRenewer(holder, &fakePersister{}, func(context.Context, *Identity) error { return nil }, RenewConfig{}, nil)
	r.after = func(time.Duration) <-chan time.Time { return make(chan time.Time) }
	client := &fakeRenewClient{ca: ca, nodeID: "node-1", validFor: 90 * 24 * time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go r.Run(ctx, client)

	r.RenewNow()
	deadline := time.Now().Add(5 * time.Second)
	for holder.Current() == id {
		if time.Now().After(deadline) {
			t.Fatal("RenewNow did not renew a certificate that was nowhere near due")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRenewer_ClockSkewBound(t *testing.T) {
	ca, _ := GenerateCA()
	tests := []struct {
		name    string
		skew    time.Duration
		wantErr bool
	}{
		{"agent clock 30 minutes behind still verifies the new certificate", -30 * time.Minute, false},
		{"agent clock 30 minutes ahead", 30 * time.Minute, false},
		{"agent clock 2 hours behind refuses a certificate that is not yet valid for it", -2 * time.Hour, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := issuedIdentity(t, ca, "node-1", time.Hour)
			holder := NewIdentityHolder(old)
			persist := &fakePersister{}
			r := NewRenewer(holder, persist, func(context.Context, *Identity) error { return nil }, RenewConfig{}, nil)
			r.now = func() time.Time { return time.Now().Add(tt.skew) }
			client := &fakeRenewClient{ca: ca, nodeID: "node-1", validFor: 90 * 24 * time.Hour}
			err := r.RenewOnce(context.Background(), client)
			if (err != nil) != tt.wantErr {
				t.Fatalf("RenewOnce() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && (holder.Current() != old || len(persist.events) != 0) {
				t.Fatal("a refused certificate must leave the old identity in use and nothing staged")
			}
		})
	}
}

func TestRenewer_HalfSucceededRenewalKeepsOldCertAndRecordsFailure(t *testing.T) {
	ca, _ := GenerateCA()
	old := issuedIdentity(t, ca, "node-1", 90*24*time.Hour)
	holder := NewIdentityHolder(old)
	persist := &fakePersister{failOn: "stage", failErr: errors.New("disk full")}
	r := NewRenewer(holder, persist, func(context.Context, *Identity) error { return nil },
		RenewConfig{Fraction: 0.5, Jitter: -1, RetryMin: time.Minute}, nil)
	leaf, _ := old.Leaf()
	r.now = func() time.Time { return leaf.NotBefore.Add(48 * 24 * time.Hour) }
	ctx, cancel := context.WithCancel(context.Background())
	r.after = func(time.Duration) <-chan time.Time { cancel(); return make(chan time.Time) }
	client := &fakeRenewClient{ca: ca, nodeID: "node-1", validFor: 90 * 24 * time.Hour}

	r.Run(ctx, client)
	if holder.Current() != old {
		t.Fatal("the control plane issued a certificate but the swap failed: the old one must stay in use")
	}
	st := r.Status()
	if st.ConsecutiveFailures != 1 || st.LastError == "" {
		t.Fatalf("status = %+v, want the persist failure recorded", st)
	}
	if client.calls != 1 {
		t.Fatalf("Renew called %d times, want 1", client.calls)
	}
}

func TestRenewer_CertificateIssuedAlreadyDueDoesNotLoop(t *testing.T) {
	ca, _ := GenerateCA()
	old := issuedIdentity(t, ca, "node-1", time.Hour)
	r := NewRenewer(NewIdentityHolder(old), &fakePersister{}, func(context.Context, *Identity) error { return nil },
		RenewConfig{Fraction: 0.3, Jitter: -1, RetryMin: time.Minute}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	var waits []time.Duration
	r.after = func(d time.Duration) <-chan time.Time {
		waits = append(waits, d)
		cancel()
		return make(chan time.Time)
	}
	client := &fakeRenewClient{ca: ca, nodeID: "node-1", validFor: time.Hour}

	r.Run(ctx, client)
	if client.calls != 1 || len(waits) != 1 || waits[0] < time.Minute {
		t.Fatalf("renewals = %d, waits = %v; want one renewal then a wait of at least RetryMin", client.calls, waits)
	}
}
