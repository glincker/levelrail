package network

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"
)

type fakeHostLookup struct {
	addrs []netip.Addr
	err   error
	calls int
}

func (f *fakeHostLookup) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	f.calls++
	return f.addrs, f.err
}

func newTestHubResolver(l *fakeHostLookup, now *time.Time) *HubHostResolver {
	return NewHubHostResolver(HubHostConfig{
		Lookup: l,
		TTL:    func() time.Duration { return 5 * time.Minute },
		Now:    func() time.Time { return *now },
	})
}

func TestHubHostResolverPrefersIPv4(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHostLookup{addrs: []netip.Addr{netip.MustParseAddr("2001:db8::1"), netip.MustParseAddr("203.0.113.7")}}
	res, err := newTestHubResolver(l, &now).Resolve(context.Background(), "cp.example.com")
	if err != nil || res.IP.String() != "203.0.113.7" {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
	l.addrs = []netip.Addr{netip.MustParseAddr("2001:db8::1")}
	now = now.Add(time.Hour)
	res, err = newTestHubResolver(l, &now).Resolve(context.Background(), "cp.example.com")
	if err != nil || res.IP.String() != "2001:db8::1" {
		t.Fatalf("AAAA fallback: res = %+v, err = %v", res, err)
	}
}

func TestHubHostResolverRejectsPrivate(t *testing.T) {
	now := time.Unix(1000, 0)
	for _, ip := range []string{"10.0.0.5", "127.0.0.1", "169.254.1.1", "192.168.1.9"} {
		l := &fakeHostLookup{addrs: []netip.Addr{netip.MustParseAddr(ip)}}
		res, err := newTestHubResolver(l, &now).Resolve(context.Background(), "cp.example.com")
		var he *HubHostError
		if !errors.As(err, &he) || he.Reason != HubHostPrivate || he.Resolved.String() != ip {
			t.Fatalf("%s: err = %v", ip, err)
		}
		if res.IP.IsValid() {
			t.Errorf("%s: a private answer must not become an endpoint", ip)
		}
	}
}

func TestHubHostResolverKeepsLastGoodOnFailure(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHostLookup{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.7")}}
	r := newTestHubResolver(l, &now)
	if _, err := r.Resolve(context.Background(), "cp.example.com"); err != nil {
		t.Fatal(err)
	}
	now = now.Add(10 * time.Minute)
	l.addrs, l.err = nil, &net.DNSError{Err: "no such host", Name: "cp.example.com", IsNotFound: true}
	res, err := r.Resolve(context.Background(), "cp.example.com")
	var he *HubHostError
	if !errors.As(err, &he) || he.Reason != HubHostNotFound {
		t.Fatalf("err = %v, want not found", err)
	}
	if res.IP.String() != "203.0.113.7" || !res.Stale {
		t.Errorf("res = %+v, want stale last good IP", res)
	}
	l.err = context.DeadlineExceeded
	now = now.Add(10 * time.Minute)
	if _, err := r.Resolve(context.Background(), "cp.example.com"); !errors.As(err, &he) || he.Reason != HubHostTimeout {
		t.Errorf("err = %v, want timeout", err)
	}
}

func TestHubHostResolverFirstFailureHasNoIP(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHostLookup{err: &net.DNSError{IsNotFound: true}}
	res, err := newTestHubResolver(l, &now).Resolve(context.Background(), "gone.example.com")
	if err == nil || res.IP.IsValid() {
		t.Fatalf("res = %+v, err = %v", res, err)
	}
}

func TestHubHostResolverIPLiteralUnchanged(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHostLookup{}
	res, err := newTestHubResolver(l, &now).Resolve(context.Background(), "198.51.100.4")
	if err != nil || res.IP.String() != "198.51.100.4" || l.calls != 0 {
		t.Fatalf("res = %+v, err = %v, lookups = %d", res, err, l.calls)
	}
}

func TestHubHostResolverHonoursTTLAndHostChange(t *testing.T) {
	now := time.Unix(1000, 0)
	l := &fakeHostLookup{addrs: []netip.Addr{netip.MustParseAddr("203.0.113.7")}}
	r := newTestHubResolver(l, &now)
	ctx := context.Background()
	_, _ = r.Resolve(ctx, "cp.example.com")
	now = now.Add(4 * time.Minute)
	_, _ = r.Resolve(ctx, "cp.example.com")
	if l.calls != 1 {
		t.Fatalf("lookups inside the TTL = %d, want 1", l.calls)
	}
	l.addrs = []netip.Addr{netip.MustParseAddr("203.0.113.8")}
	now = now.Add(2 * time.Minute)
	res, _ := r.Resolve(ctx, "cp.example.com")
	if l.calls != 2 || res.IP.String() != "203.0.113.8" {
		t.Fatalf("after the TTL: lookups = %d, ip = %s", l.calls, res.IP)
	}
	l.addrs = []netip.Addr{netip.MustParseAddr("203.0.113.9")}
	res, _ = r.Resolve(ctx, "other.example.com")
	if l.calls != 3 || res.IP.String() != "203.0.113.9" {
		t.Fatalf("host change must re-resolve: lookups = %d, ip = %s", l.calls, res.IP)
	}
}
