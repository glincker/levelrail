package golive

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/domaindoctor"
	"github.com/GLINCKER/levelrail/internal/fixit"
	"github.com/GLINCKER/levelrail/internal/hostfw"
	"github.com/GLINCKER/levelrail/internal/hostprovider"
)

var now = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

func healthy() Input {
	rep := &domaindoctor.Report{Domain: "shop.dev", Probed: true, Propagation: &domaindoctor.Propagation{State: domaindoctor.PropagationSettled}}
	return Input{
		App: "shop", Domain: "shop.dev", Now: now, Seen: NewSeen(),
		AppState: AppInput{Known: true, Running: true, Ready: true},
		Port:     PortInput{ZeroConfigURL: "https://shop.1-2-3-4.sslip.io", Probe: &PortProbe{OK: true, Status: 200}},
		Firewall: FirewallInput{Host: hostfw.Status{Backend: hostfw.BackendUFW, Installed: true, Active: true, Open: map[int]bool{80: true, 443: true}}},
		Report:   rep,
		Cert: CertInput{Ready: true, CertAPI: "POST /api/v1/domains/shop.dev/cert",
			Status: CertStatus{State: "issued", Trusted: true, Issuer: "R11", NotBefore: now.Add(-24 * time.Hour), NotAfter: now.Add(89 * 24 * time.Hour), RenewalAt: now.Add(59 * 24 * time.Hour)}},
		Live: &SelfCheckResult{Reached: true, TLSValid: true, Status: 200, Match: MatchBody, Scope: ScopeControlPlane},
	}
}

func status(cl Checklist, id string) Step {
	for _, s := range cl.Steps {
		if s.ID == id {
			return s
		}
	}
	return Step{}
}

func TestBuildStates(t *testing.T) {
	blocked := hostfw.Status{Backend: hostfw.BackendUFW, Installed: true, Active: true, Open: map[int]bool{80: false, 443: false}}
	cases := []struct {
		name   string
		mut    func(*Input)
		state  State
		step   string
		status Status
		action string
	}{
		{"everything live", func(*Input) {}, StateLive, StepLive, StatusOK, ""},
		{"no deploy yet", func(in *Input) { in.AppState = AppInput{} }, StateNotLive, StepApp, StatusPending, ActionLink},
		{"app crashed", func(in *Input) { in.AppState = AppInput{Known: true, DiagnosisHref: "/apps/shop/deploys"} }, StateNeedsAttention, StepApp, StatusFail, ActionLink},
		{"port not answering", func(in *Input) { in.Port.Probe = &PortProbe{Failure: "connection refused"} }, StateLive, StepPort, StatusFail, ActionRecheck},
		{"firewall blocking with auto fix", func(in *Input) {
			in.Firewall = FirewallInput{Host: blocked, FixID: "host.firewall.open_web_ports", FixKind: fixit.KindAuto, Plan: hostfw.PlanOpen(blocked)}
		}, StateNeedsAttention, StepFirewall, StatusFail, ActionFix},
		{"firewall blocking but guided only", func(in *Input) {
			in.Firewall = FirewallInput{Host: blocked, FixKind: fixit.KindGuided, Plan: hostfw.Plan{Refusal: "x"}}
		}, StateNeedsAttention, StepFirewall, StatusFail, ActionRecheck},
		{"host firewall fine, provider blocks port 80", func(in *Input) {
			g := hostprovider.For(hostprovider.Hetzner, "dmi")
			in.Firewall.Provider = &g
			in.Report.Checks = []domaindoctor.Check{{ID: domaindoctor.IDPort80, State: domaindoctor.StateFail, Title: "Port 80 is closed"}}
		}, StateNeedsAttention, StepFirewall, StatusFail, ActionLink},
		{"firewall unreadable", func(in *Input) {
			in.Firewall.Host = hostfw.Status{Backend: hostfw.BackendUFW, Unknown: true, Detail: "needs root"}
		}, StateLive, StepFirewall, StatusUnknown, ActionRecheck},
		{"no firewall tool", func(in *Input) { in.Firewall.Host = hostfw.Status{Backend: hostfw.BackendNone} }, StateLive, StepFirewall, StatusOK, ""},
		{"dns record missing with provider fix", func(in *Input) {
			in.Report.Checks = []domaindoctor.Check{{ID: domaindoctor.IDDNSResolves, State: domaindoctor.StateFail, Title: "No DNS record",
				Fix: &domaindoctor.Fix{Action: &domaindoctor.Action{Kind: domaindoctor.ActionRequest, Label: "Create the record", API: "POST /api/v1/apps/shop/domains/shop.dev/dns-records", Body: map[string]string{"type": "A"}}}}}
			in.Report.Records = []domaindoctor.Record{{Name: "shop.dev", Type: "A", Value: "1.2.3.4"}}
			in.Cert.Status = CertStatus{}
		}, StateWaitingDNS, StepDNS, StatusFail, ActionRequest},
		{"dns record missing, copy only", func(in *Input) {
			in.Report.Checks = []domaindoctor.Check{{ID: domaindoctor.IDDNSResolves, State: domaindoctor.StateFail, Title: "No DNS record"}}
			in.Report.Records = []domaindoctor.Record{{Name: "shop.dev", Type: "A", Value: "1.2.3.4"}}
		}, StateWaitingDNS, StepDNS, StatusFail, ActionCopy},
		{"dns propagating shows next check", func(in *Input) {
			in.Report.Checks = []domaindoctor.Check{{ID: domaindoctor.IDDNSPointsHere, State: domaindoctor.StateFail, Title: "Not everywhere yet"}}
			in.Report.Propagation = &domaindoctor.Propagation{State: domaindoctor.PropagationPropagating, NextCheckSeconds: 60}
		}, StateWaitingDNS, StepDNS, StatusPending, ActionRecheck},
		{"cloudflare proxied warns", func(in *Input) {
			in.Report.Checks = []domaindoctor.Check{{ID: domaindoctor.IDDNSProxied, State: domaindoctor.StateWarn, Title: "Proxied by Cloudflare"}}
		}, StateWaitingDNS, StepDNS, StatusWarn, ActionRecheck},
		{"cert not requested yet", func(in *Input) { in.Cert.Status = CertStatus{State: "off"} }, StateWaitingCertificate, StepCertificate, StatusPending, ActionRequest},
		{"cert preflight blocked", func(in *Input) {
			in.Cert.Status = CertStatus{State: "off"}
			in.Cert.Ready, in.Cert.Blocker = false, "No contact email"
		}, StateNeedsAttention, StepCertificate, StatusFail, ActionRecheck},
		{"cert pending", func(in *Input) { in.Cert.Status = CertStatus{State: "pending"} }, StateWaitingCertificate, StepCertificate, StatusPending, ActionRecheck},
		{"cert failed retry", func(in *Input) {
			in.Cert.Status = CertStatus{State: "failed", Error: "x", Explanation: "Port 80 unreachable"}
		}, StateNeedsAttention, StepCertificate, StatusFail, ActionRequest},
		{"cert failed rate limited", func(in *Input) {
			n := now.Add(time.Hour)
			in.Cert.Status = CertStatus{State: "failed", Error: "x", NextSafeAt: &n}
		}, StateNeedsAttention, StepCertificate, StatusFail, ActionRecheck},
		{"internal certificate", func(in *Input) { in.Cert.Status = CertStatus{State: "issued", Trusted: false} }, StateWaitingCertificate, StepCertificate, StatusWarn, ActionRequest},
		{"waiting on dns blocks certificate", func(in *Input) {
			in.Cert.Status = CertStatus{State: "off"}
			in.Report.Checks = []domaindoctor.Check{{ID: domaindoctor.IDDNSResolves, State: domaindoctor.StateFail}}
		}, StateWaitingDNS, StepCertificate, StatusPending, ActionRecheck},
		{"live check tls invalid", func(in *Input) { in.Live = &SelfCheckResult{Reached: true, TLSError: "not trusted"} }, StateNeedsAttention, StepLive, StatusFail, ActionRecheck},
		{"live check unreachable", func(in *Input) { in.Live = &SelfCheckResult{Error: "refused"} }, StateNeedsAttention, StepLive, StatusFail, ActionRecheck},
		{"live check other server", func(in *Input) {
			in.Live = &SelfCheckResult{Reached: true, TLSValid: true, Status: 404, Match: MatchNone}
		}, StateNeedsAttention, StepLive, StatusWarn, ActionRecheck},
		{"live check not run", func(in *Input) { in.Live = nil }, StateWaitingCertificate, StepLive, StatusUnknown, ActionRecheck},
		{"no domain yet", func(in *Input) { in.Domain, in.Report = "", nil }, StateWaitingDNS, StepDNS, StatusPending, ActionLink},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := healthy()
			tc.mut(&in)
			cl := Build(in)
			s := status(cl, tc.step)
			if s.Status != tc.status || cl.State != tc.state {
				t.Fatalf("state=%s step %s=%s (%s), want %s / %s", cl.State, tc.step, s.Status, s.Summary, tc.state, tc.status)
			}
			if tc.status != StatusOK && (s.Primary == nil || s.Primary.Kind != tc.action) {
				t.Fatalf("primary = %+v, want %s", s.Primary, tc.action)
			}
			if cl.Live != (tc.state == StateLive) {
				t.Fatal("Live flag disagrees with state")
			}
		})
	}
}

func TestLiveStepIsHonestAboutScope(t *testing.T) {
	cl := Build(healthy())
	s := status(cl, StepLive)
	if s.Scope != ScopeControlPlane || !strings.Contains(s.Evidence[0].Value, "cannot prove") {
		t.Fatalf("live step does not state its scope: %+v", s)
	}
}

func TestPortStepWarnsAboutDirectPort(t *testing.T) {
	in := healthy()
	in.Port.DirectURL = "http://1.2.3.4:24100"
	s := status(Build(in), StepPort)
	if !strings.Contains(s.Summary, "only for testing") || !s.Evidence[1].Copy {
		t.Fatalf("direct port step = %+v", s)
	}
}

func TestPendingCertSetsNextCheck(t *testing.T) {
	in := healthy()
	in.Cert.Status = CertStatus{State: "pending"}
	in.PollEvery = 7 * time.Second
	s := status(Build(in), StepCertificate)
	if s.NextCheckAt == nil || !s.NextCheckAt.Equal(now.Add(7*time.Second)) {
		t.Fatalf("next check = %v", s.NextCheckAt)
	}
}

func TestTimelineOrderAndFailures(t *testing.T) {
	in := healthy()
	in.ChallengeStartedAt = now.Add(-48 * time.Hour)
	in.Failure = &Failure{At: now.Add(-47 * time.Hour), Error: "e", Explanation: "port 80 unreachable"}
	cl := Build(in)
	var kinds []string
	for i, e := range cl.Timeline {
		kinds = append(kinds, e.Kind)
		if i > 0 && e.At.Before(cl.Timeline[i-1].At) {
			t.Fatalf("timeline not ordered: %v", cl.Timeline)
		}
	}
	for _, want := range []string{KindChallenge, KindFailure, KindCert, KindRenewal, KindDNS, KindLive} {
		if !contains(kinds, want) {
			t.Fatalf("missing %s in %v", want, kinds)
		}
	}
	if !cl.Timeline[len(cl.Timeline)-1].Future {
		t.Fatalf("renewal should be last and scheduled: %+v", cl.Timeline)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestSeenKeepsFirstObservation(t *testing.T) {
	s := NewSeen()
	a := s.Mark("k", now)
	b := s.Mark("k", now.Add(time.Hour))
	if !a.Equal(b) {
		t.Fatal("first observation must stick")
	}
	s.Forget("k")
	if c := s.Mark("k", now.Add(time.Hour)); c.Equal(a) {
		t.Fatal("forget should reset")
	}
}

func TestSelfCheck(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello from the app"))
	}))
	defer srv.Close()
	pool := x509.NewCertPool()
	pool.AddCert(srv.Certificate())
	addr := strings.TrimPrefix(srv.URL, "https://")
	bodyHash := HashBody(strings.NewReader("hello from the app"))
	cases := []struct {
		name      string
		in        SelfCheckInput
		reached   bool
		tlsValid  bool
		match     Match
		tlsErrHas string
	}{
		{"same body", SelfCheckInput{Domain: "example.com", Addr: addr, RootCAs: pool, Reference: &Reference{Status: 200, BodyHash: bodyHash}}, true, true, MatchBody, ""},
		{"same headers, different body", SelfCheckInput{Domain: "example.com", Addr: addr, RootCAs: pool, Reference: &Reference{Status: 200, ContentType: "text/plain", BodyHash: "x"}}, true, true, MatchHeaders, ""},
		{"another server answers", SelfCheckInput{Domain: "example.com", Addr: addr, RootCAs: pool, Reference: &Reference{Status: 404}}, true, true, MatchNone, ""},
		{"no reference", SelfCheckInput{Domain: "example.com", Addr: addr, RootCAs: pool}, true, true, MatchUnverified, ""},
		{"certificate for another name", SelfCheckInput{Domain: "other.test", Addr: addr, RootCAs: pool}, true, false, MatchNone, "does not cover"},
		{"untrusted issuer", SelfCheckInput{Domain: "example.com", Addr: addr}, true, false, MatchNone, "authority"},
		{"nothing listening", SelfCheckInput{Domain: "example.com", Addr: "127.0.0.1:1", Timeout: time.Second}, false, false, MatchNone, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := SelfCheck(context.Background(), tc.in)
			if r.Reached != tc.reached || r.TLSValid != tc.tlsValid || r.Match != tc.match || !strings.Contains(r.TLSError, tc.tlsErrHas) || r.Scope != ScopeControlPlane {
				t.Fatalf("%+v", r)
			}
		})
	}
}
