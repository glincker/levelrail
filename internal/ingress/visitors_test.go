package ingress

import (
	"net/netip"
	"testing"
	"time"
)

func TestSummarizeVisitors(t *testing.T) {
	l := NewAccessLog(100)
	now := time.Now()
	add := func(host, ip, path, ref string, status int, ago time.Duration) {
		l.Record("k", AccessRecord{Time: now.Add(-ago), Host: host, Path: path, Status: status, Referrer: ref, Client: netip.MustParseAddr(ip)})
	}
	add("a.test:443", "10.0.0.1", "/", "news.test", 200, time.Minute)
	add("a.test", "10.0.0.1", "/x", "", 404, 2*time.Minute)
	add("a.test", "10.0.0.2", "/", "news.test", 200, 3*time.Minute)
	add("b.test", "10.0.0.3", "/", "", 502, time.Minute)
	all := l.Summarize([]string{"k"}, VisitorSummaryOptions{Since: now.Add(-time.Hour), Until: now})
	if all.Requests != 4 || all.Visitors != 3 || all.Status["5xx"] != 1 || all.Status["4xx"] != 1 {
		t.Fatalf("all: %+v", all)
	}
	one := l.Summarize([]string{"k"}, VisitorSummaryOptions{Domain: "a.test", Since: now.Add(-time.Hour), Until: now})
	if one.Requests != 3 || one.Visitors != 2 || one.TopPaths[0].Label != "/" || one.TopRefs[0].Count != 2 || len(one.Series) != visitorSeriesBuckets {
		t.Fatalf("domain: %+v", one)
	}
	if h1, h2 := l.visitorHash("10.0.0.1", now, time.Hour), l.visitorHash("10.0.0.1", now.Add(2*time.Hour), time.Hour); h1 == h2 {
		t.Fatal("hash must change when the salt period rolls")
	}
}
