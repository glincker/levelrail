package edgefw

import (
	"net/netip"
	"testing"
	"time"
)

func TestBansNeverBanCloudflareAddress(t *testing.T) {
	l := DefaultLimits()
	l.BanStrikes = 1
	b := NewBans(l)
	cf, other := netip.MustParseAddr("173.245.48.9"), netip.MustParseAddr("198.51.100.9")
	now := time.Now()
	if b.Strike(cf, "rule", now) {
		t.Fatal("a Cloudflare address was banned")
	}
	b.Add(cf, "manual", time.Minute, now)
	if b.Active(cf, now) {
		t.Fatal("a Cloudflare address is banned")
	}
	if !b.Strike(other, "rule", now) {
		t.Fatal("other addresses must still be banned")
	}
}
