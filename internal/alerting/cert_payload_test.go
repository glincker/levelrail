package alerting

import (
	"strings"
	"testing"
	"time"
)

func TestCertPolicy_StructuredPayload(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	notAfter := now.Add(2*oneDay + time.Hour)
	info := CertInfo{Domain: "app.example.com", Issuer: "R3", NotBefore: now.Add(-60 * oneDay), NotAfter: notAfter}
	cases := []struct {
		name     string
		acme     ACMEFailureSource
		wantKind string
	}{
		{"dns failure", stubACME{"app.example.com": "no valid A records found"}, CertFailureDNS},
		{"other acme failure", stubACME{"app.example.com": "rate limited by the CA"}, CertFailureRenewalFailing},
		{"no acme failure", nil, CertFailureOther},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fact, alert := testPolicy(tc.acme, 3*oneDay).assess(info, now, time.Hour, false)
			if !alert {
				t.Fatal("expected an alert")
			}
			p := fact.Payload()
			if p.FailureKind != tc.wantKind {
				t.Errorf("failure kind = %q, want %q", p.FailureKind, tc.wantKind)
			}
			if p.Domain != "app.example.com" || p.Issuer != "R3" || p.DaysLeft != 2 || !p.NotAfter.Equal(notAfter) {
				t.Errorf("payload = %+v", p)
			}
		})
	}
}

func TestCertPayload_StoredBesideDetail(t *testing.T) {
	fact := CertFact{Domain: "a.example.com", Issuer: "R3", DaysLeft: 4, Reason: "Automatic renewal is overdue", NotAfter: time.Unix(1700000000, 0)}
	cases := []struct {
		name       string
		ev         Event
		detail     string
		wantCert   bool
		wantDetail string
	}{
		{"firing keeps text and adds data", Event{CertFacts: []CertFact{fact}}, "", true, "a.example.com: expires in 4 days (issuer R3). Automatic renewal is overdue."},
		{"existing detail is kept", Event{CertFacts: []CertFact{fact}}, "grouped", true, "grouped"},
		{"resolved carries no payload", Event{Resolved: true, CertFacts: []CertFact{fact}}, "", false, ""},
		{"non cert event has none", Event{}, "", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := HistoryEntry{Detail: tc.detail}
			certHistory(tc.ev)(&e)
			if (e.Cert != nil) != tc.wantCert {
				t.Fatalf("cert set = %v, want %v", e.Cert != nil, tc.wantCert)
			}
			if e.Detail != tc.wantDetail {
				t.Errorf("detail = %q, want %q", e.Detail, tc.wantDetail)
			}
			if !tc.wantCert {
				return
			}
			raw, err := encodeCertPayload(e.Cert)
			if err != nil || !strings.Contains(raw, `"failure_kind":"other"`) {
				t.Fatalf("encoded = %q, err = %v", raw, err)
			}
			got := decodeCertPayload(raw)
			if got == nil || got.Domain != "a.example.com" || got.DaysLeft != 4 {
				t.Errorf("round trip = %+v", got)
			}
		})
	}
}

func TestDecodeCertPayload_Tolerant(t *testing.T) {
	for _, raw := range []string{"", "not json", `{"days_left":3}`} {
		if got := decodeCertPayload(raw); got != nil {
			t.Errorf("decodeCertPayload(%q) = %+v, want nil", raw, got)
		}
	}
}
