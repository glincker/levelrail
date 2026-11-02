package domaincutover

import "testing"

func okProbe() Probe { return Probe{Done: true, Status: 200, CertTrusted: true, CertIssuer: "R3"} }

func base() Facts {
	return Facts{
		Domain: "glinr.com", App: okProbe(), CertState: CertIssued, CertIssuer: "R3",
		DNSStatus: DNSElsewhere, Resolved: []string{"198.51.100.4"}, ExpectedIPv4: []string{"203.0.113.7"}, TTLSeconds: 60,
	}
}

func states(r Result) map[string]State {
	m := map[string]State{}
	for _, s := range r.Steps {
		m[s.ID] = s.State
	}
	return m
}

func TestEvaluateStepStates(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Facts)
		risk   Risk
		want   map[string]State
	}{
		{"ready, cert issued, not yet switched", func(*Facts) {}, RiskNone,
			map[string]State{StepApp: StatePass, StepCertificate: StatePass, StepCurrentDNS: StatePass, StepSwitch: StateWait, StepAuthoritative: StateWait, StepLive: StateWait}},
		{"cert pending", func(f *Facts) { f.App.CertTrusted = false; f.CertState = CertPending }, RiskLikely,
			map[string]State{StepApp: StatePass, StepCertificate: StateWait}},
		{"cert failed", func(f *Facts) {
			f.App.CertTrusted = false
			f.CertState = CertFailed
			f.CertExplanation = "the CA could not reach the challenge"
		}, RiskLikely,
			map[string]State{StepCertificate: StateFail}},
		{"cert never requested", func(f *Facts) { f.App.CertTrusted = false; f.CertState = CertOff }, RiskLikely,
			map[string]State{StepCertificate: StateWait}},
		{"app answers 502", func(f *Facts) { f.App = Probe{Done: true, Status: 502, Err: "bad gateway"} }, RiskLikely,
			map[string]State{StepApp: StateFail}},
		{"app not probed yet", func(f *Facts) { f.App = Probe{} }, RiskUnknown,
			map[string]State{StepApp: StateWait}},
		{"proxy owns tls", func(f *Facts) { f.UpstreamTLS = true; f.App.CertTrusted = false; f.CertState = CertOff }, RiskNone,
			map[string]State{StepCertificate: StateSkip}},
		{"high ttl asks to lower it", func(f *Facts) { f.TTLSeconds = 86400 }, RiskNone,
			map[string]State{StepCurrentDNS: StateWait}},
		{"unknown ttl asks to lower it", func(f *Facts) { f.TTLSeconds = 0 }, RiskNone,
			map[string]State{StepCurrentDNS: StateWait}},
		{"switched, authoritative partial", func(f *Facts) {
			f.DNSStatus, f.Resolved = DNSConnected, []string{"203.0.113.7"}
			f.AuthKnown, f.AuthTotal, f.AuthHit = true, 2, 1
		}, RiskNone, map[string]State{StepSwitch: StatePass, StepAuthoritative: StateWait, StepLive: StateWait}},
		{"fully live", func(f *Facts) {
			f.DNSStatus, f.Resolved = DNSConnected, []string{"203.0.113.7"}
			f.AuthKnown, f.AuthTotal, f.AuthHit = true, 2, 2
			f.Live = okProbe()
		}, RiskNone, map[string]State{StepCurrentDNS: StatePass, StepSwitch: StatePass, StepAuthoritative: StatePass, StepLive: StatePass}},
		{"live probe untrusted", func(f *Facts) {
			f.DNSStatus, f.Resolved = DNSConnected, []string{"203.0.113.7"}
			f.AuthKnown, f.AuthTotal, f.AuthHit = true, 1, 1
			f.Live = Probe{Done: true, Status: 200}
		}, RiskNone, map[string]State{StepLive: StateFail}},
		{"cloudflare proxied", func(f *Facts) { f.Proxied = true; f.Resolved = []string{"104.16.1.1"} }, RiskNone,
			map[string]State{StepCurrentDNS: StatePass}},
		{"not resolving at all", func(f *Facts) { f.DNSStatus, f.Resolved, f.TTLSeconds = DNSNotResolving, nil, 0 }, RiskNone,
			map[string]State{StepCurrentDNS: StatePass, StepSwitch: StateWait}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := base()
			c.mutate(&f)
			r := Evaluate(f, Options{})
			if r.Risk != c.risk {
				t.Errorf("risk = %s, want %s (%s)", r.Risk, c.risk, r.Summary)
			}
			got := states(r)
			for id, want := range c.want {
				if got[id] != want {
					t.Errorf("step %s = %s, want %s", id, got[id], want)
				}
			}
			if len(r.Steps) != 6 {
				t.Fatalf("steps = %d", len(r.Steps))
			}
		})
	}
}

func TestEvaluateCertificateActionByProvider(t *testing.T) {
	f := base()
	f.App.CertTrusted, f.CertState = false, CertOff
	if a := Evaluate(f, Options{}).Steps[1].Action; a == nil || a.Kind != "link" {
		t.Fatalf("no provider: action = %+v, want the runbook link", a)
	}
	f.ProviderConnected = true
	r := Evaluate(f, Options{})
	if a := r.Steps[1].Action; a == nil || a.Kind != "request" || a.API != "POST /api/v1/domains/glinr.com/cutover/certificate" {
		t.Fatalf("provider: action = %+v", a)
	}
	if len(r.CLI) != 2 {
		t.Errorf("cli = %v", r.CLI)
	}
}

func TestEvaluateRecordsAndProxyNote(t *testing.T) {
	f := base()
	f.Proxied, f.ExpectedIPv6 = true, []string{"2001:db8::7"}
	r := Evaluate(f, Options{LowTTL: 120})
	if len(r.DNS.Records) != 2 || r.DNS.Records[0].Type != "A" || r.DNS.Records[1].Type != "AAAA" || r.DNS.Records[0].TTLSeconds != 120 {
		t.Fatalf("records = %+v", r.DNS.Records)
	}
	if r.ProxyDocs != CloudflareDocs || r.DNS.Points != PointsCloudflare {
		t.Fatalf("proxy = %q points = %q", r.ProxyDocs, r.DNS.Points)
	}
	f = base()
	f.ExpectedIPv4, f.ExpectedHost = nil, "lr.example.net"
	if rec := Evaluate(f, Options{}).DNS.Records; len(rec) != 1 || rec[0].Type != "CNAME" {
		t.Fatalf("cname records = %+v", rec)
	}
}

func TestSummaryWording(t *testing.T) {
	f := base()
	f.App.CertTrusted, f.CertState = false, CertPending
	want := "Ready to switch: the app answers on this server for glinr.com (certificate pending)"
	if got := Evaluate(f, Options{}).Summary; got != want {
		t.Errorf("summary = %q", got)
	}
}
