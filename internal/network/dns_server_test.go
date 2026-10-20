package network

// These tests query a real listener over real UDP on a loopback port,
// which needs no root, no TUN device, and no mesh: it is the DNS protocol
// behavior, not the networking, that is under test here. The record
// derivation those answers come from is tested separately in dns_test.go.

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func startTestDNS(t *testing.T, r *Resolver) *DNSServer {
	t.Helper()
	srv := NewDNSServer(r, quietLogger())
	// Start does not return until both listeners are actually serving,
	// so no polling for readiness is needed here.
	if err := srv.Start(context.Background(), "127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		if err := srv.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	if !srv.Addr().IsValid() {
		t.Fatal("server never reported a bound address")
	}
	return srv
}

func query(t *testing.T, srv *DNSServer, name string, qtype uint16) *dns.Msg {
	t.Helper()
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)

	c := &dns.Client{Timeout: 3 * time.Second}
	resp, _, err := c.Exchange(m, srv.Addr().String())
	if err != nil {
		t.Fatalf("querying %s for %s: %v", srv.Addr(), name, err)
	}
	return resp
}

func TestDNSServer_AnswersARecords(t *testing.T) {
	zone := Zone("acme")
	r := NewResolver(zone)
	set, err := BuildRecords(zone, "control", []Placement{{Service: "postgres", NodeID: "worker"}}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	r.SetRecords(set)

	srv := startTestDNS(t, r)
	resp := query(t, srv, "postgres."+zone, dns.TypeA)

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("Rcode = %s, want NOERROR", dns.RcodeToString[resp.Rcode])
	}
	if !resp.Authoritative {
		t.Error("response is not marked authoritative")
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("got %d answers, want 1", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("answer is %T, want *dns.A", resp.Answer[0])
	}
	if !a.A.Equal(net.ParseIP("10.181.0.2")) {
		t.Errorf("answer = %s, want 10.181.0.2", a.A)
	}
	if a.Hdr.Ttl != uint32(dnsRecordTTL.Seconds()) {
		t.Errorf("TTL = %d, want %d", a.Hdr.Ttl, int(dnsRecordTTL.Seconds()))
	}
}

// TestDNSServer_MovedServiceResolvesToTheNewNode is the moved-service
// case again, this time end to end over the wire, because a resolver that
// is correct in memory and stale on the wire is the same outage.
func TestDNSServer_MovedServiceResolvesToTheNewNode(t *testing.T) {
	zone := Zone("acme")
	r := NewResolver(zone)
	srv := startTestDNS(t, r)

	before, err := BuildRecords(zone, "control", []Placement{{Service: "postgres"}}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	r.SetRecords(before)

	resp := query(t, srv, "postgres."+zone, dns.TypeA)
	if len(resp.Answer) != 1 || !resp.Answer[0].(*dns.A).A.Equal(net.ParseIP("10.181.0.1")) {
		t.Fatalf("before the move: %v", resp.Answer)
	}

	after, err := BuildRecords(zone, "control", []Placement{{Service: "postgres", NodeID: "worker"}}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	r.SetRecords(after)

	resp = query(t, srv, "postgres."+zone, dns.TypeA)
	if len(resp.Answer) != 1 {
		t.Fatalf("after the move: got %d answers, want 1", len(resp.Answer))
	}
	if !resp.Answer[0].(*dns.A).A.Equal(net.ParseIP("10.181.0.2")) {
		t.Errorf("after the move: answer = %s, want 10.181.0.2", resp.Answer[0].(*dns.A).A)
	}
}

func TestDNSServer_Rcodes(t *testing.T) {
	zone := Zone("acme")
	r := NewResolver(zone)
	set, err := BuildRecords(zone, "control", []Placement{{Service: "web"}}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	r.SetRecords(set)
	srv := startTestDNS(t, r)

	tests := []struct {
		name      string
		query     string
		qtype     uint16
		wantRcode int
		wantEmpty bool
	}{
		{
			name:      "unknown name in our zone is NXDOMAIN",
			query:     "nothing." + zone,
			qtype:     dns.TypeA,
			wantRcode: dns.RcodeNameError,
		},
		{
			// REFUSED, not NXDOMAIN: the client must know to ask someone
			// else rather than conclude the name does not exist anywhere.
			name:      "a name outside our zone is REFUSED",
			query:     "example.com",
			qtype:     dns.TypeA,
			wantRcode: dns.RcodeRefused,
		},
		{
			// The mesh is IPv4 only. NXDOMAIN here would make a
			// dual-stack client that asks AAAA first give up before
			// asking for the A record.
			name:      "AAAA for an existing name is an empty NOERROR",
			query:     "web." + zone,
			qtype:     dns.TypeAAAA,
			wantRcode: dns.RcodeSuccess,
			wantEmpty: true,
		},
		{
			name:      "AAAA for an unknown name is still NXDOMAIN",
			query:     "nothing." + zone,
			qtype:     dns.TypeAAAA,
			wantRcode: dns.RcodeNameError,
		},
		{
			name:      "an unsupported type for an existing name is an empty NOERROR",
			query:     "web." + zone,
			qtype:     dns.TypeMX,
			wantRcode: dns.RcodeSuccess,
			wantEmpty: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := query(t, srv, tc.query, tc.qtype)
			if resp.Rcode != tc.wantRcode {
				t.Errorf("Rcode = %s, want %s",
					dns.RcodeToString[resp.Rcode], dns.RcodeToString[tc.wantRcode])
			}
			if tc.wantEmpty && len(resp.Answer) != 0 {
				t.Errorf("got %d answers, want none", len(resp.Answer))
			}
		})
	}
}

func TestDNSServer_LifecycleFailures(t *testing.T) {
	r := NewResolver(Zone("acme"))
	srv := NewDNSServer(r, quietLogger())

	if err := srv.Start(context.Background(), "127.0.0.1:0"); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := srv.Start(context.Background(), "127.0.0.1:0"); err == nil {
		t.Error("starting an already-started server was allowed")
	}

	if err := srv.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("second Close must be a no-op, got %v", err)
	}
	if err := srv.Start(context.Background(), "127.0.0.1:0"); err == nil {
		t.Error("a closed server was restarted")
	}
}

func TestDNSServer_StartFailures(t *testing.T) {
	r := NewResolver(Zone("acme"))

	t.Run("unbindable address", func(t *testing.T) {
		srv := NewDNSServer(r, quietLogger())
		// 203.0.113.0/24 is TEST-NET-3, which is not assigned to any
		// local interface, so binding it must fail.
		if err := srv.Start(context.Background(), "203.0.113.1:5353"); err == nil {
			_ = srv.Close()
			t.Fatal("binding an address this host does not hold succeeded")
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		srv := NewDNSServer(r, quietLogger())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := srv.Start(ctx, "127.0.0.1:0"); err == nil {
			_ = srv.Close()
			t.Fatal("a cancelled context did not stop the server from starting")
		}
	})
}
