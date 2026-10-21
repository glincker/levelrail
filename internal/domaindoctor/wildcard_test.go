package domaindoctor

import (
	"context"
	"strings"
	"testing"
	"time"
)

type noWildcardDNS struct{ fakeDNS }

func (n noWildcardDNS) Answers(ctx context.Context, host string) []ResolverAnswer {
	if strings.HasPrefix(host, "probe-") {
		return []ResolverAnswer{{Resolver: "this server", Error: "not found"}}
	}
	return n.fakeDNS.Answers(ctx, host)
}

func TestWildcardAbsent(t *testing.T) {
	if got := check(t, runWith(t, baseFacts(), fakeDNS{}), IDDNSWildcard); got.State != StateSkipped {
		t.Fatalf("unresolved domain: %+v", got)
	}
	tc, hc := new(int), new(int)
	probes := Probes{DNS: noWildcardDNS{fakeDNS{answers: here()}}, TLS: fakeTLS{cert: goodCert(), calls: tc}, HTTP: fakeHTTP{responses: redirectToHTTPS(), calls: hc, dialed: new([]string)}}
	rep := Run(context.Background(), baseFacts(), probes, Options{Now: func() time.Time { return testNow }})
	if c := check(t, rep, IDDNSWildcard); c.State != StatePass || !strings.Contains(c.Detail, "no wildcard record") {
		t.Fatalf("wildcard = %+v", c)
	}
}
