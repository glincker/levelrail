package pages

import "testing"

func TestRefHostAndMerge(t *testing.T) {
	if got := RefHost("https://user:pw@WWW.Example.com:8443/a?q=1#x"); got != "example.com" {
		t.Fatalf("host = %q", got)
	}
	out := MergeReferrers([]Count{{"example.com", 2}, {"https://www.example.com/x?y=1", 1}, {"", 5}, {"b.io", 1}}, 5)
	if len(out) != 2 || out[0].Label != "example.com" || out[0].Count != 3 {
		t.Fatalf("merge = %+v", out)
	}
	st := StatusBreakdown([]int{200, 200, 404, 99})
	if len(st) != 2 || st[0].Code != 200 || st[0].Count != 2 {
		t.Fatalf("status = %+v", st)
	}
	if RangeFits(30*24*3600e9, 7) {
		t.Fatal("30d must not fit 7d retention")
	}
}
