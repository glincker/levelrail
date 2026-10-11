package dnszones

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func TestBuildPlanAndApply(t *testing.T) {
	existing := []RecordSet{
		{Name: Apex, Type: "NS", TTL: 300, Values: []string{"ns1.x"}},
		{Name: "www", Type: "A", TTL: 300, Values: []string{"1.1.1.1"}},
		{Name: "old", Type: "TXT", TTL: 300, Values: []string{"bye"}},
		{Name: "same", Type: "A", TTL: 300, Values: []string{"2.2.2.2"}},
	}
	desired := []RecordSet{
		{Name: "www", Type: "A", TTL: 300, Values: []string{"9.9.9.9"}},
		{Name: "same", Type: "A", TTL: 300, Values: []string{"2.2.2.2"}},
		{Name: "new", Type: "CNAME", TTL: 300, Values: []string{"x.net"}},
	}
	tests := []struct {
		name    string
		replace bool
		summary map[string]int
	}{
		{"merge never deletes", false, map[string]int{ActionUpdate: 1, ActionUnchanged: 1, ActionCreate: 1}},
		{"replace deletes foreign but keeps apex NS", true, map[string]int{ActionUpdate: 1, ActionUnchanged: 1, ActionCreate: 1, ActionDelete: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := BuildPlan(existing, desired, tt.replace, Capabilities{})
			for k, v := range tt.summary {
				if p.Summary[k] != v {
					t.Errorf("summary[%s] = %d, want %d (%v)", k, p.Summary[k], v, p.Summary)
				}
			}
			if p.Blocked {
				t.Fatalf("unexpected block: %+v", p.Changes)
			}
			fp := &fakeProvider{sets: slices.Clone(existing)}
			n, err := ApplyPlan(context.Background(), fp, Zone{}, p)
			if err != nil {
				t.Fatal(err)
			}
			if want := len(p.Changes) - p.Summary[ActionUnchanged]; n != want {
				t.Errorf("applied %d, want %d", n, want)
			}
			if tt.replace && !strings.HasPrefix(fp.calls[0], "delete") {
				t.Errorf("deletes must run first, calls = %v", fp.calls)
			}
		})
	}
}

func TestBuildPlanBlocksConflicts(t *testing.T) {
	existing := []RecordSet{{Name: "www", Type: "A", Values: []string{"1.1.1.1"}}}
	p := BuildPlan(existing, []RecordSet{{Name: "www", Type: "CNAME", Values: []string{"x.net"}}}, false, Capabilities{})
	if !p.Blocked {
		t.Fatal("CNAME over an existing A must block the plan")
	}
	if _, err := ApplyPlan(context.Background(), &fakeProvider{}, Zone{}, p); err == nil {
		t.Fatal("ApplyPlan must refuse a blocked plan")
	}
	// With replace, the A is deleted in the same plan, so no conflict remains.
	if p := BuildPlan(existing, []RecordSet{{Name: "www", Type: "CNAME", Values: []string{"x.net"}}}, true, Capabilities{}); p.Blocked {
		t.Fatalf("replace plan should not block: %+v", p.Changes)
	}
	if p := BuildPlan(nil, []RecordSet{{Name: "x", Type: "A", Proxied: true, Values: []string{"1.1.1.1"}}}, false, Capabilities{}); !p.Blocked {
		t.Fatal("proxied on a provider without proxy support must block")
	}
}

func TestZoneFileRoundTrip(t *testing.T) {
	sets := []RecordSet{
		{Name: Apex, Type: "A", TTL: 300, Values: []string{"203.0.113.1"}},
		{Name: Apex, Type: "MX", TTL: 3600, Values: []string{"10 mail.example.com", "20 mx2.example.net"}},
		{Name: Apex, Type: "TXT", TTL: 300, Values: []string{"v=spf1 mx ~all", strings.Repeat("k", 400)}},
		{Name: Apex, Type: "CAA", TTL: 300, Values: []string{`0 issue "letsencrypt.org"`}},
		{Name: "*", Type: "A", TTL: 60, Values: []string{"203.0.113.2"}},
		{Name: "www", Type: "CNAME", TTL: 300, Values: []string{"example.com"}},
		{Name: "_sip._tls", Type: "SRV", TTL: 300, Values: []string{"100 1 443 sip.example.net"}},
		{Name: "dev", Type: "NS", TTL: 300, Values: []string{"ns1.other.net"}},
		{Name: "v6", Type: "AAAA", TTL: 300, Values: []string{"2001:db8::1"}},
		{Name: "lb", Type: "A", TTL: 60, SetIdentifier: "a", Routing: RoutingWeighted, Weight: w(1), Values: []string{"1.1.1.1"}},
	}
	text := ExportBIND("example.com", sets)
	if !strings.Contains(text, "; skipped lb A") {
		t.Errorf("routed set should be a comment:\n%s", text)
	}
	got, warnings, err := ParseBIND(text, "example.com", 300)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, text)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings: %v", warnings)
	}
	for i := range got {
		n, err := Normalize(got[i], "example.com", 300)
		if err != nil {
			t.Fatalf("normalize %+v: %v", got[i], err)
		}
		got[i] = n
	}
	want := slices.DeleteFunc(slices.Clone(sets), func(s RecordSet) bool { return s.SetIdentifier != "" })
	if len(got) != len(want) {
		t.Fatalf("got %d sets, want %d:\n%+v", len(got), len(want), got)
	}
	for _, ws := range want {
		i := slices.IndexFunc(got, func(g RecordSet) bool { return g.Key() == ws.Key() })
		if i < 0 || !SameContent(got[i], ws) {
			t.Errorf("set %s %s lost in round trip: %+v", ws.Name, ws.Type, got)
		}
	}

	js, err := ExportJSON("example.com", sets)
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseJSON(js)
	if err != nil || len(back) != len(sets) {
		t.Fatalf("json round trip: %v, %d sets", err, len(back))
	}
	if arr, err := ParseJSON([]byte(`[{"name":"a","type":"A","values":["1.1.1.1"]}]`)); err != nil || len(arr) != 1 {
		t.Fatalf("bare array: %v", err)
	}
}

func TestParseBINDSkips(t *testing.T) {
	text := `$ORIGIN example.com.
$TTL 600
@ IN SOA ns1.example.com. admin.example.com. 1 7200 3600 1209600 300
@ IN NS ns1.example.com.
www IN A 1.2.3.4
www IN A 1.2.3.5
host IN PTR x.example.com.
other.net. IN A 1.1.1.1
`
	sets, warnings, err := ParseBIND(text, "example.com", 300)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || len(sets[0].Values) != 2 || sets[0].TTL != 600 {
		t.Fatalf("sets = %+v", sets)
	}
	if len(warnings) != 4 {
		t.Errorf("warnings = %v", warnings)
	}
	if _, _, err := ParseBIND("www IN A not-an-ip", "example.com", 300); err == nil {
		t.Error("bad zone file must fail")
	}
}

func TestComputeDelegation(t *testing.T) {
	expected := []string{"ana.ns.cloudflare.com.", "bob.ns.cloudflare.com"}
	cf := []string{"ana.ns.cloudflare.com", "bob.ns.cloudflare.com"}
	old := []string{"ns1.registrar.net", "ns2.registrar.net"}
	tests := []struct {
		name      string
		answers   []Answer
		want      string
		elsewhere int
	}{
		{"all match", []Answer{{Values: cf}, {Values: cf}, {Values: cf}}, DelegationDelegated, 0},
		{"subset still delegated", []Answer{{Values: cf[:1]}, {Values: cf}}, DelegationDelegated, 0},
		{"one resolver lags", []Answer{{Values: cf}, {Values: old}}, DelegationPartial, 2},
		{"mixed set", []Answer{{Values: append(slices.Clone(cf), old[0])}}, DelegationPartial, 1},
		{"elsewhere", []Answer{{Values: old}, {Values: old}}, DelegationElsewhere, 2},
		{"nothing", []Answer{{Rcode: "nxdomain"}, {Error: "timeout"}}, DelegationNone, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := ComputeDelegation(expected, tt.answers)
			if d.State != tt.want || len(d.Elsewhere) != tt.elsewhere {
				t.Errorf("state = %s elsewhere = %v, want %s / %d", d.State, d.Elsewhere, tt.want, tt.elsewhere)
			}
		})
	}
}

func TestCheckDelegationAndPropagation(t *testing.T) {
	fq := &fakeQuerier{answers: map[string]Answer{
		qkey("1.1.1.1:53", "example.com", dns.TypeNS):           {Values: []string{"a.ns.x"}},
		qkey("8.8.8.8:53", "example.com", dns.TypeNS):           {Values: []string{"a.ns.x"}},
		qkey("1.1.1.1:53", "www.example.com", dns.TypeA):        {Values: []string{"1.2.3.4"}, TTL: 120},
		qkey("8.8.8.8:53", "www.example.com", dns.TypeA):        {Values: []string{"5.6.7.8"}, TTL: 30},
		qkey("a.ns.x:53", "www.example.com", dns.TypeA):         {Values: []string{"1.2.3.4"}, TTL: 300, Authoritative: true},
		qkey("a.ns.x:53", "mail.example.com", dns.TypeCNAME):    {Values: []string{"ghs.google.com"}, TTL: 300},
		qkey("a.ns.x:53", "example.com", dns.TypeMX):            {Values: []string{"1 smtp.google.com"}, TTL: 300},
		qkey("a.ns.x:53", "custom.example.com", dns.TypeTXT):    {Values: []string{"hello"}, TTL: 60},
		qkey("a.ns.x:53", "_sip._tls.example.com", dns.TypeSRV): {Values: []string{"1 1 443 sip.x"}, TTL: 60},
	}}
	ctx := context.Background()
	d := CheckDelegation(ctx, fq, "example.com", []string{"a.ns.x"}, []string{"1.1.1.1", "8.8.8.8:53"})
	if d.State != DelegationDelegated || d.Domain != "example.com" {
		t.Errorf("delegation = %+v", d)
	}

	p, err := CheckPropagation(ctx, fq, "www.example.com", "a", []string{"1.1.1.1", "8.8.8.8"}, []string{"a.ns.x"}, []string{"1.2.3.4"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Agree || len(p.Answers) != 3 || p.Answers[2].Source != SourceAuthoritative {
		t.Errorf("propagation = %+v", p)
	}
	if m := p.Answers[1].Matches; m == nil || *m {
		t.Errorf("8.8.8.8 should not match: %+v", p.Answers[1])
	}
	if _, err := CheckPropagation(ctx, fq, "x", "PTR", nil, nil, nil); err == nil {
		t.Error("unsupported type must fail")
	}

	found := Discover(ctx, fq, "example.com", []string{"custom"}, []string{"a.ns.x"})
	var got []string
	for _, s := range found {
		got = append(got, s.Name+" "+s.Type)
	}
	for _, want := range []string{"@ MX", "mail CNAME", "custom TXT", "_sip._tls SRV", "www A"} {
		if !slices.Contains(got, want) {
			t.Errorf("discover missing %q in %v", want, got)
		}
	}
	for _, k := range fq.asked {
		if strings.HasPrefix(k, "a.ns.x:53|mail.example.com|A") {
			t.Error("a name with a CNAME must not be probed for other types")
		}
	}
}

func TestTemplates(t *testing.T) {
	for _, tpl := range Templates {
		params := map[string]string{}
		for _, p := range tpl.Params {
			params[p.Key] = "x" + p.Key
		}
		params["mx_host"] = "mail.example.com"
		params["mx_prefix"] = "example-com"
		params["selector"] = "s1"
		params["policy"] = "none"
		sets, err := tpl.Render(params)
		if err != nil {
			t.Fatalf("%s: %v", tpl.ID, err)
		}
		for _, s := range sets {
			if _, err := Normalize(s, "example.com", 300); err != nil {
				t.Errorf("%s renders an invalid set %+v: %v", tpl.ID, s, err)
			}
		}
	}
	tpl, ok := FindTemplate("email")
	if !ok {
		t.Fatal("email template missing")
	}
	if _, err := tpl.Render(nil); err == nil {
		t.Error("missing required param must fail")
	}
	existing := []RecordSet{{Name: Apex, Type: "TXT", TTL: 900, Values: []string{"google-site-verification=abc", "v=spf1 -all"}}}
	merged := MergeTemplate(existing, []RecordSet{{Name: Apex, Type: "TXT", Values: []string{"v=spf1 mx ~all"}}})
	if len(merged) != 1 || !slices.Equal(merged[0].Values, []string{"v=spf1 mx ~all", "google-site-verification=abc"}) || merged[0].TTL != 900 {
		t.Errorf("merged = %+v", merged)
	}
}
