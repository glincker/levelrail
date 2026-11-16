package network

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func cacheMsg(name string, ttl uint32) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)
	m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl}, A: net.ParseIP("192.0.2.1")}}
	return m
}

func TestDNSCache_ClampsTTLAndHonorsViews(t *testing.T) {
	c := newDNSCache(func() int { return 1 << 20 }, func() time.Duration { return 10 * time.Second }, func() time.Duration { return time.Minute })
	now := time.Now()
	c.now = func() time.Time { return now }
	k := cacheKey{name: "a.test.", qtype: dns.TypeA, qclass: dns.ClassINET}
	if !c.put(k, cacheMsg("a.test.", 3600)) {
		t.Fatal("not stored")
	}
	if c.get(cacheKey{name: k.name, qtype: k.qtype, qclass: k.qclass, view: "other"}) != nil {
		t.Fatal("answer leaked across views")
	}
	now = now.Add(11 * time.Second)
	if c.get(k) != nil {
		t.Fatal("entry outlived the clamped TTL")
	}
}

func TestDNSCache_StaysInsideByteBudget(t *testing.T) {
	budget := 8 * 1024
	c := newDNSCache(func() int { return budget }, func() time.Duration { return time.Hour }, func() time.Duration { return time.Hour })
	for i := 0; i < 500; i++ {
		name := fmt.Sprintf("h%d.test.", i)
		c.put(cacheKey{name: name, qtype: dns.TypeA, qclass: dns.ClassINET}, cacheMsg(name, 300))
	}
	total := 0
	for i := range c.shards {
		total += c.shards[i].used
	}
	if total > budget || total == 0 {
		t.Fatalf("cache holds %d bytes, budget %d", total, budget)
	}
}

func TestDNSCache_OffWhenBudgetZero(t *testing.T) {
	c := newDNSCache(func() int { return 0 }, func() time.Duration { return time.Hour }, func() time.Duration { return time.Hour })
	if c.put(cacheKey{name: "a.test."}, cacheMsg("a.test.", 60)) {
		t.Fatal("stored with a zero budget")
	}
}
