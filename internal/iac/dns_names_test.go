package iac

import (
	"context"
	"strings"
	"testing"
)

const dnsNamesDoc = "    dns:\n      names: [shop, www.shop]\n"

func TestDNSNamesRoundTripAndClear(t *testing.T) {
	f := newFake()
	opts := Options{Source: "git"}
	src := strings.Replace(roundTripApp, "%s", dnsNamesDoc, 1)
	applyAll(t, f, src, opts)
	if _, ok := f.apps["api"]["dns_names"]; !ok {
		t.Fatalf("apply did not save dns_names: %v", f.apps["api"])
	}
	if p := planFor(t, f, src, opts); quiet(p) != 0 {
		t.Fatalf("replan not empty: %+v", p.Changes)
	}
	exp, err := Export(context.Background(), f, ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exp.Join(), "dns:") || !strings.Contains(exp.Join(), "www.shop") {
		t.Fatalf("export lacks dns names:\n%s", exp.Join())
	}
	if p := planFor(t, f, exp.Join(), opts); quiet(p) != 0 {
		t.Fatalf("export does not replan clean: %+v", p.Changes)
	}

	cleared := strings.Replace(roundTripApp, "%s", "    dns:\n      names: []\n", 1)
	if p := planFor(t, f, cleared, opts); p.Summary.Update != 1 {
		t.Fatalf("plan missed the clear: %+v", p.Summary)
	}
	bare := strings.Replace(roundTripApp, "%s", "", 1)
	if p := planFor(t, f, bare, opts); p.Summary.Update != 0 {
		t.Fatalf("undeclared dns must stay unmanaged: %+v", p.Changes)
	}
}

func TestDNSNamesInvalidAndConflict(t *testing.T) {
	f := newFake()
	opts := Options{Source: "git"}
	bad := strings.Replace(roundTripApp, "%s", "    dns:\n      names: [\"Bad_Name\"]\n", 1)
	if issues := buildIssues(t, bad); len(issues) == 0 {
		t.Fatal("invalid name must be an issue")
	}
	one := strings.Replace(roundTripApp, "%s", dnsNamesDoc, 1)
	two := strings.Replace(strings.Replace(roundTripApp, "name: api", "name: other", 1), "%s", dnsNamesDoc, 1)
	p := planFor(t, f, one+"\n---\n"+two, opts)
	found := false
	for _, c := range p.Changes {
		found = found || c.Code == CodeDNSNameConflict
	}
	if !found {
		t.Fatalf("want %s: %+v", CodeDNSNameConflict, p.Changes)
	}
}
