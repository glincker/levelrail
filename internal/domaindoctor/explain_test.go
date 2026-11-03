package domaindoctor

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsinspect"
	"github.com/GLINCKER/levelrail/internal/fixit"
)

// Built from code points so this file itself passes the dash sweep.
var bannedDashes = string([]rune{0x2013, 0x2014})

// emittedCheckIDs reads the ID constants out of the package source, so a new
// check cannot ship without an explanation.
func emittedCheckIDs(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !strings.HasPrefix(n.Name, "ID") || !ok || lit.Kind != token.STRING {
						continue
					}
					v, _ := strconv.Unquote(lit.Value)
					ids = append(ids, v)
				}
			}
		}
	}
	if len(ids) < 15 {
		t.Fatalf("found only %d check id constants", len(ids))
	}
	return ids
}

func looksLikeCommand(s string) bool {
	for _, p := range []string{"$ ", "sudo ", "levelrail", "docker ", "dig ", "curl ", "systemctl "} {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func fixitIDs() map[string]bool {
	out := map[string]bool{}
	for _, id := range fixit.ExplainIDs() {
		if e, _ := fixit.Explain(id); e.FixID != "" {
			out[e.FixID] = true
		}
	}
	return out
}

func TestEveryCheckHasFix(t *testing.T) {
	t.Parallel()
	known := fixitIDs()
	for _, id := range emittedCheckIDs(t) {
		e, ok := explainTable[id]
		if !ok {
			t.Errorf("check %s has no explanation", id)
			continue
		}
		if e.fail == "" && e.warn == "" || e.label == "" {
			t.Errorf("%s: cause or label is empty", id)
		}
		for _, cause := range []string{e.fail, e.warn} {
			if cause != "" && (!strings.HasSuffix(cause, ".") || strings.ContainsAny(cause, bannedDashes) || strings.Contains(cause, "NXDOMAIN")) {
				t.Errorf("%s: cause %q must be one plain sentence", id, cause)
			}
		}
		if e.fixID != "" && !known[e.fixID] {
			t.Errorf("%s: fix id %q does not exist in fixit", id, e.fixID)
		}
		for _, p := range []string{dnsinspect.ProviderCloudflare, dnsinspect.ProviderRoute53, dnsinspect.ProviderGoDaddy, dnsinspect.ProviderOther} {
			steps := e.steps(guideCtx{domain: "shop.example.com", app: "shop", provider: p})
			if len(steps) == 0 {
				t.Errorf("%s/%s: no steps", id, p)
			}
			plain := false
			for _, s := range steps {
				if strings.ContainsAny(s, bannedDashes) {
					t.Errorf("%s: step %q has a banned dash", id, s)
				}
				plain = plain || !looksLikeCommand(s)
			}
			if !plain {
				t.Errorf("%s/%s: the only content is a command", id, p)
			}
		}
	}
}

func TestProviderGuidedSteps(t *testing.T) {
	t.Parallel()
	cases := []struct{ provider, want string }{
		{dnsinspect.ProviderCloudflare, "Cloudflare, choose your domain"},
		{dnsinspect.ProviderRoute53, "Route 53, choose Hosted zones"},
		{dnsinspect.ProviderGoDaddy, "GoDaddy, open My Products"},
		{dnsinspect.ProviderOther, "company that runs your domain's name servers"},
		{dnsinspect.ProviderUnknown, "company that runs your domain's name servers"},
	}
	for _, c := range cases {
		steps := explainTable[IDDNSResolves].steps(guideCtx{domain: "shop.example.com", provider: c.provider, rec: &Record{Name: "shop.example.com", Type: "A", Value: "203.0.113.7"}})
		if !strings.Contains(steps[0], c.want) || !strings.Contains(steps[1], "A record named shop.example.com with the value 203.0.113.7") {
			t.Errorf("%s steps = %q", c.provider, steps)
		}
	}
}

func TestEvidenceFormatting(t *testing.T) {
	t.Parallel()
	rn := &run{facts: Facts{Domain: "shop.example.com", ServerIPs: []string{serverIP}, PortHolders: []PortHolder{{Port: 80, Container: "edge", Kind: "proxy"}}}}
	rn.answers = []ResolverAnswer{
		{Resolver: "1.1.1.1", IPv4: []string{"198.51.100.9"}},
		{Resolver: "8.8.8.8", Error: "timeout"},
		{Resolver: "authoritative ns1.example.com"},
	}
	got := rn.evidence(&Check{ID: IDDNSResolves, Detail: "d"})
	want := []string{"1.1.1.1 answered 198.51.100.9", "8.8.8.8 failed: timeout", "authoritative ns1.example.com had no answer", "Server address expected: " + serverIP, "d"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("dns evidence = %q", got)
	}
	if got := rn.evidence(&Check{ID: IDProxyPorts}); len(got) != 1 || got[0] != "Port 80 is held by edge (proxy)" {
		t.Errorf("port evidence = %q", got)
	}
	rn.tls = &PeerCert{Subject: "CN=x", Issuer: "CN=R11", DNSNames: []string{"a.example.com"}, NotAfter: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)}
	got = rn.evidence(&Check{ID: IDTLS})
	if len(got) != 2 || got[0] != "Certificate is for a.example.com, issued by CN=R11" || got[1] != "Certificate ends on 2026-12-01" {
		t.Errorf("tls evidence = %q", got)
	}
	rn.facts.ACMEFailure = &ACMEFailureFacts{Error: strings.Repeat("x", 500)}
	got = rn.evidence(&Check{ID: IDStoredCert})
	if len(got[0]) > maxEvidenceLine+3 {
		t.Errorf("evidence line not clipped: %d", len(got[0]))
	}
}

func TestRunAddsExplanation(t *testing.T) {
	t.Parallel()
	dns := fakeDNS{answers: []ResolverAnswer{
		{Resolver: "this server", Error: "not found"},
		{Resolver: "authoritative aria.ns.cloudflare.com", Error: "no answer", Authoritative: true},
		{Resolver: "authoritative bob.ns.cloudflare.com", Error: "no answer", Authoritative: true},
	}}
	facts := baseFacts()
	rep := Run(context.Background(), facts, Probes{DNS: dns}, Options{Now: func() time.Time { return testNow }})
	for _, c := range rep.Checks {
		if c.State != StateFail && c.State != StateWarn {
			continue
		}
		if c.Cause == "" || len(c.Evidence) == 0 || c.Fix == nil || len(c.Fix.Steps) == 0 {
			t.Errorf("%s (%s) lacks cause, evidence or steps: %+v", c.ID, c.State, c)
		}
	}
	if c := rep.Checks[0]; c.ID != IDDNSResolves || !strings.Contains(c.Fix.Steps[0], "Cloudflare") {
		t.Errorf("first check = %+v", c)
	}
}
