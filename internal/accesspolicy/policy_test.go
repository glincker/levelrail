package accesspolicy

import (
	"errors"
	"net/netip"
	"testing"
	"time"
)

func TestMerge(t *testing.T) {
	r, err := Merge([]Policy{
		{IdleTimeout: time.Hour, MaxLifetime: 0, IPAllowlist: []string{"10.0.0.0/8"}},
		{IdleTimeout: 15 * time.Minute, MaxLifetime: 12 * time.Hour, RequireMFA: true, IPAllowlist: []string{"10.1.0.0/16", "192.0.2.7"}},
		{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.IdleTimeout != 15*time.Minute || r.MaxLifetime != 12*time.Hour || !r.RequireMFA || len(r.AllowLists) != 2 {
		t.Fatalf("rules = %+v", r)
	}
	if _, err := Merge([]Policy{{IPAllowlist: []string{"10.0.0.0/33"}}}); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("bad cidr: %v", err)
	}
	if _, err := Merge([]Policy{{IPAllowlist: []string{"example.com"}}}); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("hostname: %v", err)
	}
}

func TestIPAllowed(t *testing.T) {
	r, _ := Merge([]Policy{{IPAllowlist: []string{"10.0.0.0/8", "2001:db8::/32"}}, {IPAllowlist: []string{"10.1.0.0/16", "2001:db8::/32"}}})
	for ip, want := range map[string]bool{
		"10.1.2.3":         true,
		"10.2.0.1":         false,
		"192.0.2.1":        false,
		"2001:db8::1":      true,
		"::ffff:10.1.2.3":  true,
		"::ffff:192.0.2.9": false,
	} {
		if got := r.IPAllowed(netip.MustParseAddr(ip)); got != want {
			t.Errorf("IPAllowed(%s) = %v", ip, got)
		}
	}
	if r.IPAllowed(netip.Addr{}) {
		t.Error("invalid address passed a non-empty allowlist")
	}
	if !(Rules{}).IPAllowed(netip.Addr{}) {
		t.Error("no allowlist must allow everyone")
	}
}

func TestSessionAge(t *testing.T) {
	now := time.Now()
	r := Rules{IdleTimeout: 10 * time.Minute, MaxLifetime: 8 * time.Hour}
	for _, tc := range []struct {
		name          string
		created, seen time.Time
		wasSeen       bool
		want          Verdict
	}{
		{"fresh", now.Add(-time.Minute), now, true, Allowed},
		{"idle", now.Add(-time.Hour), now.Add(-11 * time.Minute), true, ExpiredIdle},
		{"never seen counts from creation", now.Add(-time.Hour), time.Time{}, false, ExpiredIdle},
		{"never seen but new", now.Add(-time.Minute), time.Time{}, false, Allowed},
		{"too old", now.Add(-9 * time.Hour), now, true, ExpiredLifetime},
	} {
		if got := r.SessionAge(now, tc.created, tc.seen, tc.wasSeen); got != tc.want {
			t.Errorf("%s: %q", tc.name, got)
		}
	}
	if (Rules{}).SessionAge(now, now.Add(-1000*time.Hour), now.Add(-1000*time.Hour), true) != Allowed {
		t.Error("zero rules expired a session")
	}
}

func TestClientAddr(t *testing.T) {
	trusted := func(a netip.Addr) bool { return a.IsLoopback() || a == netip.MustParseAddr("10.0.0.5") }
	for _, tc := range []struct {
		peer, xff, want string
	}{
		{"203.0.113.9", "198.51.100.1", "203.0.113.9"},
		{"127.0.0.1", "", "127.0.0.1"},
		{"127.0.0.1", "198.51.100.1", "198.51.100.1"},
		{"127.0.0.1", "6.6.6.6, 198.51.100.1, 10.0.0.5", "198.51.100.1"},
		{"127.0.0.1", "garbage", "invalid IP"},
	} {
		got := ClientAddr(netip.MustParseAddr(tc.peer), tc.xff, trusted)
		if got.String() != tc.want {
			t.Errorf("ClientAddr(%s, %q) = %s want %s", tc.peer, tc.xff, got, tc.want)
		}
	}
}
