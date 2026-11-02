package domaindoctor

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsinspect"
)

func src(name string, addrs ...string) dnsinspect.Source {
	return dnsinspect.Source{Name: name, Addresses: addrs}
}

func hasHint(v DNSVerdict, code string) bool {
	for _, h := range v.Hints {
		if h.Code == code {
			return true
		}
	}
	return false
}

func TestJudgeDNSAuthoritativeBeatsStaleLocalCloudflare(t *testing.T) {
	now := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	v := JudgeDNS(DNSVerdictInput{
		Expected: []string{"72.61.8.70"}, Now: now, Budget: 10 * time.Minute, FlushCommand: "resolvectl flush-caches",
		Inspection: dnsinspect.Inspection{AuthReachable: true,
			Authoritative: []dnsinspect.Source{src("ns1", "72.61.8.70")}, Public: []dnsinspect.Source{src("1.1.1.1", "72.61.8.70")}},
		Local: dnsinspect.Source{Name: "this server", Addresses: []string{"104.21.17.53", "2606:4700::6810:1135"}, TTL: 600},
	})
	if v.Verdict != VerdictReady || !v.LocalStale {
		t.Fatalf("verdict=%s stale=%v", v.Verdict, v.LocalStale)
	}
	if want := now.Add(10 * time.Minute); !v.RetryAt.Equal(want) {
		t.Errorf("retry at %v, want local ttl %v", v.RetryAt, want)
	}
	if !hasHint(v, HintLocalStale) || !hasHint(v, HintWasCloudflare) {
		t.Errorf("hints = %+v", v.Hints)
	}
}

func TestJudgeDNSEscalatesAfterBudget(t *testing.T) {
	in := DNSVerdictInput{Expected: []string{"72.61.8.70"}, Now: time.Now(), Budget: 10 * time.Minute, Waiting: 11 * time.Minute,
		Inspection: dnsinspect.Inspection{Zone: "example.com", AuthReachable: true, DelegatedNS: []string{"a.ns"}, ZoneNS: []string{"b.ns"},
			Authoritative: []dnsinspect.Source{{Name: "a.ns", Error: "no records"}}}}
	v := JudgeDNS(in)
	if v.Verdict != VerdictNotFound || !v.Escalated {
		t.Fatalf("verdict=%s escalated=%v", v.Verdict, v.Escalated)
	}
	for _, c := range []string{HintCheckZone, HintCheckAAAA, HintCheckCNAME, HintCheckNameserver, HintNSMismatch} {
		if !hasHint(v, c) {
			t.Errorf("missing hint %s", c)
		}
	}
	in.Waiting = time.Minute
	if v := JudgeDNS(in); v.Escalated || !hasHint(v, HintWaiting) {
		t.Errorf("within budget must stay calm: %+v", v)
	}
}

func TestJudgeDNSFallsBackToPublicWhenAuthoritativeUnreachable(t *testing.T) {
	v := JudgeDNS(DNSVerdictInput{Expected: []string{"72.61.8.70"}, Now: time.Now(), FallbackWait: 5 * time.Minute,
		Inspection: dnsinspect.Inspection{Public: []dnsinspect.Source{src("1.1.1.1", "72.61.8.70")}},
		Local:      dnsinspect.Source{Addresses: []string{"1.2.3.4"}}})
	if v.Verdict != VerdictPublicReady || !v.LocalStale {
		t.Fatalf("verdict=%s stale=%v", v.Verdict, v.LocalStale)
	}
	if time.Until(v.RetryAt) < 4*time.Minute {
		t.Errorf("retry at %v, want the fallback wait", v.RetryAt)
	}
}

func TestJudgeDNSElsewhereAndUnknown(t *testing.T) {
	v := JudgeDNS(DNSVerdictInput{Expected: []string{"72.61.8.70"}, Now: time.Now(),
		Inspection: dnsinspect.Inspection{AuthReachable: true, Authoritative: []dnsinspect.Source{src("ns", "1.1.1.1", "2001:db8::1")}}})
	if v.Verdict != VerdictElsewhere || !hasHint(v, HintAAAAElsewhere) {
		t.Errorf("verdict=%s hints=%+v", v.Verdict, v.Hints)
	}
	if v := JudgeDNS(DNSVerdictInput{Expected: []string{"72.61.8.70"}, Now: time.Now()}); v.Verdict != VerdictUnknown {
		t.Errorf("no data must be unknown, got %s", v.Verdict)
	}
}
