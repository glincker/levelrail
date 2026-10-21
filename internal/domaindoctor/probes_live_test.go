package domaindoctor

import (
	"context"
	"testing"
	"time"
)

func TestNetDNSResolvesWellKnownName(t *testing.T) {
	if testing.Short() {
		t.Skip("needs outbound DNS")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	answers := NetDNS{}.Answers(ctx, "example.com")
	t.Logf("resolved example.com in %s", time.Since(start).Round(time.Millisecond))
	ok := false
	for _, a := range answers {
		ok = ok || len(a.IPv4)+len(a.IPv6) > 0
	}
	if !ok {
		t.Skipf("no resolver answered (offline?): %+v", answers)
	}
}
