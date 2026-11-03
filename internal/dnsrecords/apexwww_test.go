package dnsrecords

import (
	"errors"
	"testing"
	"time"

	"github.com/libdns/libdns"
)

func TestApexWWWPlan(t *testing.T) {
	tgt := Desired{Type: "A", Value: "203.0.113.7"}
	rr := func(name, typ, data string) libdns.Record { return libdns.RR{Name: name, Type: typ, Data: data} }
	cases := []struct {
		name     string
		mode     string
		existing []libdns.Record
		replace  bool
		want     []string
	}{
		{"both empty zone", WWWBoth, nil, false, []string{"@:A:created", "www:CNAME:created"}},
		{"both already right", WWWBoth, []libdns.Record{rr("@", "A", "203.0.113.7"), rr("www", "CNAME", "example.dev")}, false, []string{"@:A:unchanged", "www:CNAME:unchanged"}},
		{"www points elsewhere", WWWBoth, []libdns.Record{rr("www", "A", "198.51.100.1")}, false, []string{"@:A:created", "www:CNAME:conflict"}},
		{"www replaced", WWWBoth, []libdns.Record{rr("www", "A", "198.51.100.1")}, true, []string{"@:A:created", "www:CNAME:updated"}},
		{"apex only", WWWApexOnly, nil, false, []string{"@:A:created"}},
		{"www only", WWWOnly, []libdns.Record{rr("@", "A", "198.51.100.1")}, false, []string{"www:A:created"}},
	}
	for _, c := range cases {
		items, err := PlanApexWWW("example.dev", c.mode, tgt, c.existing, c.replace, time.Minute)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got []string
		for _, it := range items {
			got = append(got, it.Desired.Name+":"+it.Desired.Type+":"+it.Plan.Outcome)
		}
		if len(got) != len(c.want) {
			t.Fatalf("%s: got %v want %v", c.name, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: got %v want %v", c.name, got, c.want)
			}
		}
	}
	if _, err := PlanApexWWW("example.dev", "nope", tgt, nil, false, 0); err == nil {
		t.Error("unknown mode accepted")
	}
	if _, err := PlanApexWWW("example.dev", WWWBoth, Desired{}, nil, false, 0); !errors.Is(err, ErrNoTarget) {
		t.Errorf("no target: %v", err)
	}
}

func TestCanonicalRedirect(t *testing.T) {
	cases := []struct{ mode, canon, from, to string }{
		{WWWBoth, CanonicalApex, "www.example.dev", "example.dev"},
		{WWWBoth, CanonicalWWW, "example.dev", "www.example.dev"},
		{WWWApexOnly, CanonicalApex, "", ""},
		{WWWBoth, "bogus", "", ""},
	}
	for _, c := range cases {
		f, to := CanonicalRedirect("example.dev", c.mode, c.canon)
		if f != c.from || to != c.to {
			t.Errorf("%v: got %q -> %q", c, f, to)
		}
	}
}
