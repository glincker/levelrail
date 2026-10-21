package alerting

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/mailtemplates"
)

const oneDay = 24 * time.Hour

func seedACME(t *testing.T, f *fakeCertSource, notBefore, notAfter time.Time) {
	t.Helper()
	domain := "app.example.com"
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test ACME CA"}, NotBefore: notBefore, NotAfter: notAfter.Add(oneDay), IsCA: true}
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain}, NotBefore: notBefore, NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	f.certs["certificates/acme-v02.api.letsencrypt.org-directory/"+domain+"/"+domain+".crt"] = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

type stubACME map[string]string

func (f stubACME) ACMEFailing(domain string) (string, bool) {
	msg, ok := f[domain]
	return msg, ok
}

func testPolicy(acme ACMEFailureSource, windows ...time.Duration) CertExpiryPolicy {
	p := CertPolicyFromEnv(func(string) string { return "" })
	p.Windows, p.ACME = windows, acme
	return p
}

func TestCertPolicy_OneWindowPerCertificate(t *testing.T) {
	now := time.Now()
	windows := []time.Duration{14 * oneDay, 7 * oneDay, 3 * oneDay, oneDay}
	cases := []struct {
		name      string
		left      time.Duration
		wantOwner time.Duration
	}{
		{"two days left belongs to the 3 oneDay rule", 2 * oneDay, 3 * oneDay},
		{"five days left belongs to the 7 oneDay rule", 5 * oneDay, 7 * oneDay},
		{"ten days left belongs to the 14 oneDay rule", 10 * oneDay, 14 * oneDay},
		{"twelve hours left belongs to the 1 oneDay rule", 12 * time.Hour, oneDay},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			certs := newFakeCertSource()
			seedACME(t, certs, now.Add(-60*oneDay), now.Add(tc.left))
			obs := newFakeCertExpiryObservationStore()
			owners := 0
			for _, w := range windows {
				r := Rule{ID: "r-" + w.String(), Kind: KindCertExpiry, ForDuration: w}
				_, res, err := EvaluateCertExpiryPolicy(context.Background(), certs, obs, r, w, time.Hour, now, nil, testPolicy(nil, windows...))
				if err != nil {
					t.Fatalf("evaluate: %v", err)
				}
				if len(res.Facts) > 0 {
					owners++
					if w != tc.wantOwner {
						t.Errorf("window %s reported the certificate, want %s", w, tc.wantOwner)
					}
				}
			}
			if owners != 1 {
				t.Errorf("rules reporting the certificate = %d, want exactly 1", owners)
			}
		})
	}
}

func TestCertPolicy_PlatformManagedCertificatesAreSilent(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name       string
		internal   bool
		lifetime   time.Duration
		left       time.Duration
		acme       stubACME
		wantAlert  bool
		wantAction string
	}{
		{"internal self-signed never alerts", true, 90 * oneDay, 2 * oneDay, nil, false, ""},
		{"short-lived cert before its renewal point", false, 7 * oneDay, 5 * oneDay, nil, false, ""},
		{"long-lived cert whose renewal is overdue", false, 90 * oneDay, 10 * oneDay, nil, true, CertActionRenew},
		{"renewal failing on dns", false, 7 * oneDay, 5 * oneDay, stubACME{"app.example.com": "DNS problem: NXDOMAIN"}, true, CertActionDNS},
		{"renewal failing for another reason", false, 7 * oneDay, 5 * oneDay, stubACME{"app.example.com": "rate limited"}, true, CertActionRenew},
		{"last resort window alerts even before the failure shows", false, 7 * oneDay, oneDay, nil, true, CertActionRenew},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			certs := newFakeCertSource()
			if tc.internal {
				certs.seed(t, "app.example.com", now.Add(-(tc.lifetime - tc.left)), now.Add(tc.left))
			} else {
				seedACME(t, certs, now.Add(-(tc.lifetime - tc.left)), now.Add(tc.left))
			}
			var acme ACMEFailureSource
			if tc.acme != nil {
				acme = tc.acme
			}
			_, res, err := EvaluateCertExpiryPolicy(context.Background(), certs, newFakeCertExpiryObservationStore(),
				Rule{ID: "r", Kind: KindCertExpiry}, 14*oneDay, time.Hour, now, nil, testPolicy(acme, 14*oneDay))
			if err != nil {
				t.Fatalf("evaluate: %v", err)
			}
			if got := len(res.Facts) > 0; got != tc.wantAlert {
				t.Fatalf("alert = %v, want %v (notices %v)", got, tc.wantAlert, res.Notices)
			}
			if tc.wantAlert && res.Facts[0].Action != tc.wantAction {
				t.Errorf("action = %q, want %q", res.Facts[0].Action, tc.wantAction)
			}
		})
	}
}

func certEngine(t *testing.T, certs *fakeCertSource, rules *fakeRuleStore, spy *spyNotifier, clock *time.Time, policy bool) *Engine {
	t.Helper()
	e := NewEngine(rules, nil, nil, nil, certs, nil, 14*oneDay, time.Hour, nil, 0, 0, nil, 0, 0, nil, nil, 0, nil, 0, func(Rule) Notifier { return spy }, nil)
	e.clock = func() time.Time { return *clock }
	if policy {
		e.SetCertExpiryPolicy(testPolicy(nil))
	}
	return e
}

func threeRules() []Rule {
	var out []Rule
	for _, d := range []time.Duration{14 * oneDay, 7 * oneDay, 3 * oneDay} {
		out = append(out, Rule{ID: "r" + d.String(), Name: "Certificate expires", Kind: KindCertExpiry, Enabled: true, ForDuration: d})
	}
	return out
}

func TestEngine_CertExpiry_OneNotificationPerCertificate(t *testing.T) {
	cases := []struct {
		name   string
		policy bool
		want   int
	}{
		{"legacy behaviour notifies once per matching window", false, 3},
		{"policy notifies only for the tightest window", true, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			certs := newFakeCertSource()
			seedACME(t, certs, now.Add(-60*oneDay), now.Add(2*oneDay))
			spy := &spyNotifier{}
			e := certEngine(t, certs, newFakeRuleStore(threeRules()...), spy, &now, tc.policy)
			if err := e.Tick(context.Background()); err != nil {
				t.Fatalf("tick: %v", err)
			}
			if got := len(spy.calls()); got != tc.want {
				t.Errorf("notifications = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestEngine_CertExpiry_NotifiesOnceRemindsDailyAndAutoResolves(t *testing.T) {
	now := time.Now()
	certs := newFakeCertSource()
	seedACME(t, certs, now.Add(-60*oneDay), now.Add(2*oneDay))
	spy := &spyNotifier{}
	rules := newFakeRuleStore(Rule{ID: "r3", Name: "Certificate expires", Kind: KindCertExpiry, Enabled: true, ForDuration: 3 * oneDay})
	e := certEngine(t, certs, rules, spy, &now, true)

	steps := []struct {
		name    string
		advance time.Duration
		renew   bool
		want    int
		resolve bool
	}{
		{"first tick notifies", 0, false, 1, false},
		{"next tick stays quiet", time.Minute, false, 1, false},
		{"just under a oneDay stays quiet", 23 * time.Hour, false, 1, false},
		{"a oneDay later sends one reminder", 2 * time.Hour, false, 2, false},
		{"immediately after the reminder stays quiet", time.Minute, false, 2, false},
		{"renewal resolves automatically", time.Minute, true, 3, true},
	}
	for _, s := range steps {
		now = now.Add(s.advance)
		if s.renew {
			certs.certs = map[string][]byte{}
			seedACME(t, certs, now, now.Add(80*oneDay))
		}
		if err := e.Tick(context.Background()); err != nil {
			t.Fatalf("%s: tick: %v", s.name, err)
		}
		calls := spy.calls()
		if len(calls) != s.want {
			t.Fatalf("%s: notifications = %d, want %d", s.name, len(calls), s.want)
		}
		if s.resolve && !calls[len(calls)-1].Resolved {
			t.Errorf("%s: last notification should be a resolve", s.name)
		}
	}
}

func TestEngine_CertExpiry_WiderRuleResolvesSilently(t *testing.T) {
	now := time.Now()
	certs := newFakeCertSource()
	seedACME(t, certs, now.Add(-60*oneDay), now.Add(5*oneDay))
	spy := &spyNotifier{}
	e := certEngine(t, certs, newFakeRuleStore(threeRules()...), spy, &now, true)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	now = now.Add(3 * oneDay)
	if err := e.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	calls := spy.calls()
	if len(calls) != 2 {
		t.Fatalf("notifications = %d, want 2 (7 oneDay rule, then 3 oneDay rule)", len(calls))
	}
	for _, c := range calls {
		if c.Resolved {
			t.Error("superseding a wider window must not send a resolve")
		}
	}
}

func TestBuildCertLinks(t *testing.T) {
	renew := CertFact{Domain: "app.example.com", Action: CertActionRenew}
	dns := CertFact{Domain: "app.example.com", Action: CertActionDNS}
	cases := []struct {
		name      string
		base      string
		fact      CertFact
		wantEmpty bool
		wantLabel string
		wantPart  string
	}{
		{"renew link", "https://deploy.example.com/", renew, false, "Renew now", "https://deploy.example.com/domains/app.example.com?"},
		{"dns link preselects the dns fix", "https://deploy.example.com", dns, false, "Check DNS", "fix=dns"},
		{"no base url", "", renew, true, "", ""},
		{"relative base", "/dashboard", renew, true, "", ""},
		{"localhost base", "http://localhost:8080", renew, true, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l := BuildCertLinks(tc.base, tc.fact, "")
			if tc.wantEmpty {
				if l != (CertLinks{}) {
					t.Fatalf("links = %+v, want none", l)
				}
				return
			}
			if l.PrimaryLabel != tc.wantLabel || !strings.Contains(l.Primary, tc.wantPart) {
				t.Errorf("links = %+v", l)
			}
			if !strings.HasSuffix(l.Inbox, "/inbox") || !strings.Contains(l.Settings, "/settings/notification-channels") {
				t.Errorf("inbox/settings links wrong: %+v", l)
			}
			if strings.Contains(l.Primary, "token") {
				t.Error("link must not carry secrets")
			}
		})
	}
}

type captureSender struct {
	key  string
	vals map[string]string
	plan string
}

func (c *captureSender) Send(_ context.Context, _, subject, body string) error {
	c.plan = subject + "\n" + body
	return nil
}

func (c *captureSender) RenderMail(_ context.Context, key, to string, vals map[string]string) (email.Message, bool) {
	c.key, c.vals = key, vals
	return email.Message{To: to, Subject: "rendered", Text: "rendered"}, true
}

func (c *captureSender) SendMessage(_ context.Context, _ email.Message) error { return nil }

func TestEmailNotifier_CertExpiryBody(t *testing.T) {
	fact := CertFact{Domain: "app.example.com", Issuer: "R3", DaysLeft: 2, Reason: "Renewal is failing: DNS problem", Action: CertActionDNS}
	cases := []struct {
		name    string
		base    string
		wantKey string
	}{
		{"with a public base url", "https://deploy.example.com", mailtemplates.EventCertExpiry},
		{"without a base url the buttons are omitted", "", mailtemplates.EventCertExpiryPlain},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &captureSender{}
			err := emailNotifier{sender: s, to: "ops@example.com"}.Notify(context.Background(), Event{
				Rule: Rule{Name: "Certificate expires"}, CertFacts: []CertFact{fact}, CertBaseURL: tc.base,
			})
			if err != nil {
				t.Fatalf("notify: %v", err)
			}
			if s.key != tc.wantKey {
				t.Fatalf("template = %q, want %q", s.key, tc.wantKey)
			}
			if s.vals["domain"] != "app.example.com" || s.vals["issuer"] != "R3" || s.vals["days_left"] != "expires in 2 days" || !strings.Contains(s.vals["reason"], "DNS") {
				t.Errorf("facts missing from values: %v", s.vals)
			}
			if tc.base == "" {
				if s.vals["primary_url"] != "" {
					t.Errorf("no base url must mean no links, got %q", s.vals["primary_url"])
				}
				return
			}
			if s.vals["primary_label"] != "Check DNS" || !strings.HasPrefix(s.vals["primary_url"], "https://deploy.example.com/domains/") {
				t.Errorf("primary link wrong: %v", s.vals)
			}
		})
	}
}
