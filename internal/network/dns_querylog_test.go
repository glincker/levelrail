package network

import (
	"net/netip"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func testQueryLog(enabled bool, size int, privacy string) *QueryLog {
	var on atomic.Bool
	on.Store(enabled)
	return NewQueryLog(QueryLogConfig{
		Enabled: on.Load,
		Size:    func() int { return size },
		Privacy: func() string { return privacy },
		AppForIP: func(a netip.Addr) string {
			if a == netip.MustParseAddr("172.18.0.5") {
				return "web"
			}
			return ""
		},
	})
}

func TestQueryLogDisabledRecordsNothing(t *testing.T) {
	l := testQueryLog(false, 10, QueryPrivacyFull)
	l.Observe(netip.MustParseAddr("172.18.0.5"), "a.acme.", "A", "NOERROR", ReasonLocalRecord, 1, time.Millisecond, time.Now())
	if got := l.Query(QueryFilter{}); len(got) != 0 {
		t.Fatalf("disabled log stored %d records", len(got))
	}
	if _, total := l.Stats(); total.Queries != 0 {
		t.Fatalf("disabled log counted %d queries", total.Queries)
	}
}

func TestQueryLogRingDropsOldestAndFilters(t *testing.T) {
	l := testQueryLog(true, 3, QueryPrivacyFull)
	src := netip.MustParseAddr("172.18.0.5")
	now := time.Now()
	for _, n := range []string{"a.", "b.", "c.", "d."} {
		l.Observe(src, n, "A", "NOERROR", ReasonLocalRecord, 1, time.Microsecond, now)
	}
	got := l.Query(QueryFilter{})
	if len(got) != 3 || got[0].QName != "d." || got[2].QName != "b." {
		t.Fatalf("ring order wrong: %+v", got)
	}
	if got := l.Query(QueryFilter{Prefix: "C", Limit: 5}); len(got) != 1 {
		t.Fatalf("prefix filter matched %d", len(got))
	}
	if got := l.Query(QueryFilter{App: "other"}); len(got) != 0 {
		t.Fatalf("app filter matched %d", len(got))
	}
	if got := l.Query(QueryFilter{Limit: 1}); len(got) != 1 {
		t.Fatalf("limit ignored: %d", len(got))
	}
}

func TestQueryLogPrivacyAppliedBeforeStorage(t *testing.T) {
	src := netip.MustParseAddr("172.18.0.5")
	cases := map[string]struct {
		wantSource  string
		wantRecords int
	}{
		QueryPrivacyFull:         {"172.18.0.5", 1},
		QueryPrivacyNamesOnly:    {"", 1},
		QueryPrivacyCountersOnly: {"", 0},
	}
	for level, want := range cases {
		l := testQueryLog(true, 10, level)
		l.Observe(src, "x.acme.", "A", "NXDOMAIN", ReasonNoSuchApp, 0, time.Microsecond, time.Now())
		got := l.Query(QueryFilter{})
		if len(got) != want.wantRecords || (len(got) == 1 && got[0].Source != want.wantSource) {
			t.Fatalf("%s: got %+v", level, got)
		}
		apps, total := l.Stats()
		if total.Queries != 1 || total.NXDomain != 1 || apps[0].App != "web" {
			t.Fatalf("%s: counters %+v %+v", level, apps, total)
		}
	}
}

func TestQueryLogResizesLive(t *testing.T) {
	size := 4
	l := NewQueryLog(QueryLogConfig{Enabled: func() bool { return true }, Size: func() int { return size }})
	for _, n := range []string{"a.", "b.", "c.", "d."} {
		l.Observe(netip.Addr{}, n, "A", "NOERROR", ReasonLocalRecord, 1, 0, time.Now())
	}
	size = 2
	l.Observe(netip.Addr{}, "e.", "A", "NOERROR", ReasonLocalRecord, 1, 0, time.Now())
	got := l.Query(QueryFilter{})
	if len(got) != 2 || got[0].QName != "e." || got[1].QName != "d." {
		t.Fatalf("after shrink: %+v", got)
	}
}

func TestDNSServerRecordsReasons(t *testing.T) {
	zone := Zone("acme")
	r := NewResolver(zone)
	set, err := BuildRecords(zone, "control", []Placement{{Service: "postgres", NodeID: "worker"}}, meshNodes())
	if err != nil {
		t.Fatalf("BuildRecords: %v", err)
	}
	r.SetRecords(set)
	srv := startTestDNS(t, r)
	l := NewQueryLog(QueryLogConfig{
		Enabled: func() bool { return true }, Size: func() int { return 10 },
		Privacy:  func() string { return QueryPrivacyFull },
		AppForIP: func(netip.Addr) string { return "client" },
	})
	srv.SetQueryLog(l)

	query(t, srv, "postgres."+zone, dns.TypeA)
	query(t, srv, "missing."+zone, dns.TypeA)
	query(t, srv, "example.org", dns.TypeA)

	reasons := map[string]string{}
	for _, q := range l.Query(QueryFilter{}) {
		reasons[q.RCode] = q.Reason
		if q.App != "client" || q.Source == "" {
			t.Fatalf("record not attributed: %+v", q)
		}
	}
	want := map[string]string{"NOERROR": ReasonLocalRecord, "NXDOMAIN": ReasonNoSuchApp, "REFUSED": ReasonRefused}
	for rc, reason := range want {
		if reasons[rc] != reason {
			t.Fatalf("rcode %s reason = %q, want %q", rc, reasons[rc], reason)
		}
	}
	if _, total := l.Stats(); total.Queries != 3 || total.NXDomain != 1 || total.Refused != 1 {
		t.Fatalf("totals %+v", total)
	}
}
