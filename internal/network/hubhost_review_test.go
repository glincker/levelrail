package network

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

type gatedLookup struct {
	mu    sync.Mutex
	ips   map[string]netip.Addr
	gate  map[string]chan struct{}
	calls int
}

func (g *gatedLookup) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	g.mu.Lock()
	g.calls++
	ch := g.gate[host]
	ip := g.ips[host]
	g.mu.Unlock()
	if ch != nil {
		<-ch
	}
	return []netip.Addr{ip}, nil
}

func TestHubHostResolverHostChangeDuringLookupDoesNotLeak(t *testing.T) {
	now := time.Unix(1000, 0)
	gate := make(chan struct{})
	l := &gatedLookup{
		ips:  map[string]netip.Addr{"a.example.com": netip.MustParseAddr("203.0.113.1"), "b.example.com": netip.MustParseAddr("203.0.113.2")},
		gate: map[string]chan struct{}{"a.example.com": gate},
	}
	r := NewHubHostResolver(HubHostConfig{Lookup: l, TTL: func() time.Duration { return 5 * time.Minute }, Now: func() time.Time { return now }})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = r.Resolve(context.Background(), "a.example.com")
	}()
	for {
		l.mu.Lock()
		n := l.calls
		l.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if res, _ := r.Resolve(context.Background(), "b.example.com"); res.IP.String() != "203.0.113.2" {
		t.Fatalf("b = %s", res.IP)
	}
	close(gate)
	<-done
	if res, _ := r.Resolve(context.Background(), "b.example.com"); res.IP.String() != "203.0.113.2" {
		t.Fatalf("after the old lookup finished, b resolved to %s", res.IP)
	}
}

func TestHubHostResolverStopsUsingStaleIPAfterStaleMax(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHostLookup{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.7")}}
	r := NewHubHostResolver(HubHostConfig{
		Lookup: l, Now: func() time.Time { return now },
		TTL:      func() time.Duration { return 5 * time.Minute },
		StaleMax: func() time.Duration { return time.Hour },
	})
	_, _ = r.Resolve(context.Background(), "cp.example.com")
	l.addrs, l.err = nil, &net.DNSError{IsNotFound: true}
	now = now.Add(30 * time.Minute)
	if res, _ := r.Resolve(context.Background(), "cp.example.com"); !res.Stale || !res.IP.IsValid() {
		t.Fatalf("inside the window: %+v", res)
	}
	now = now.Add(90 * time.Minute)
	if res, err := r.Resolve(context.Background(), "cp.example.com"); err == nil || res.IP.IsValid() {
		t.Fatalf("past the window the old IP must not be used: %+v, %v", res, err)
	}
}

func TestHubHostResolverClampsBadTTL(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHostLookup{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.7")}}
	r := NewHubHostResolver(HubHostConfig{Lookup: l, Now: func() time.Time { return now }, TTL: func() time.Duration { return 0 }})
	_, _ = r.Resolve(context.Background(), "cp.example.com")
	now = now.Add(time.Second)
	_, _ = r.Resolve(context.Background(), "cp.example.com")
	if l.calls != 1 {
		t.Fatalf("a zero TTL must not disable caching, lookups = %d", l.calls)
	}
}
