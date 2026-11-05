package certflow

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeOrderer struct {
	err   error
	calls int
	dir   string
}

func (f *fakeOrderer) ProveHTTP01(_ context.Context, directory, _ string) error {
	f.calls++
	f.dir = directory
	return f.err
}

const stagingDir = "https://acme-staging-v02.api.letsencrypt.org/directory"

func newProver(t *testing.T, o Orderer, led *Ledger) *Prover {
	t.Helper()
	p, err := NewProver(o, stagingDir, led, Limits{FailedPerHour: 3, DuplicatePerWeek: 5}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReachabilityOpen(t *testing.T) {
	o := &fakeOrderer{}
	p := newProver(t, o, &Ledger{})
	now := time.Now()
	got := p.Prove(context.Background(), "App.Example.dev", now, 5, "", time.Time{})
	if got.Outcome != OutcomeOpen || o.dir != stagingDir {
		t.Fatalf("got %+v dir %q", got, o.dir)
	}
	if again := p.Prove(context.Background(), "app.example.dev", now.Add(10*time.Second), 5, "", time.Time{}); again.Outcome != OutcomeOpen || o.calls != 1 {
		t.Fatalf("cache missed: calls %d", o.calls)
	}
}

func TestReachabilityClosedRefused(t *testing.T) {
	o := &fakeOrderer{err: &ValidationError{Detail: "dial tcp 203.0.113.9:80: connect: connection refused"}}
	got := newProver(t, o, &Ledger{}).Prove(context.Background(), "a.example.dev", time.Now(), 5, "", time.Time{})
	if got.Outcome != OutcomeClosed || !strings.Contains(got.Detail, "nothing accepted the connection") || !strings.Contains(got.CAReason, "connection refused") {
		t.Fatalf("got %+v", got)
	}
}

func TestReachabilityCAUnreachable(t *testing.T) {
	o := &fakeOrderer{err: errors.New("dial tcp: lookup acme-staging: no such host")}
	got := newProver(t, o, &Ledger{}).Prove(context.Background(), "a.example.dev", time.Now(), 5, "", time.Time{})
	if got.Outcome != OutcomeUnknown || got.CAReason != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestReachabilityLedgerLimited(t *testing.T) {
	var led Ledger
	now := time.Now()
	for i := 0; i < 3; i++ {
		led.RecordFailure("a.example.dev", now.Add(-time.Duration(i+1)*time.Minute), now)
	}
	o := &fakeOrderer{}
	got := newProver(t, o, &led).Prove(context.Background(), "a.example.dev", now, 5, "", time.Time{})
	if got.Outcome != OutcomeLimited || got.NextSafeAt == nil || o.calls != 0 {
		t.Fatalf("got %+v calls %d", got, o.calls)
	}
}

func TestReachabilityAttemptGuard(t *testing.T) {
	o := &fakeOrderer{err: errors.New("down")}
	p := newProver(t, o, &Ledger{})
	now := time.Now()
	p.Prove(context.Background(), "a.example.dev", now, 1, "", time.Time{})
	got := p.Prove(context.Background(), "a.example.dev", now.Add(2*time.Minute), 1, "", time.Time{})
	if got.Outcome != OutcomeLimited || o.calls != 1 {
		t.Fatalf("got %+v calls %d", got, o.calls)
	}
}

func TestReachabilityRefusesProduction(t *testing.T) {
	if _, err := NewProver(&fakeOrderer{}, "https://acme-v02.api.letsencrypt.org/directory", &Ledger{}, Limits{}, time.Minute); err == nil {
		t.Fatal("production directory accepted")
	}
}
