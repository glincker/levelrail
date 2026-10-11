package alerting

import (
	"context"
	"testing"
	"time"
)

func TestSuppressedCertSourceSilencesRules(t *testing.T) {
	now := time.Now().UTC()
	soon := now.Add(48 * time.Hour)
	src := newFakeCertSource()
	src.seed(t, "app.example.com", now.Add(-time.Hour), soon)
	src.seed(t, "other.example.com", now.Add(-time.Hour), soon)
	only := func(skip func(string) bool) CertSuppressor {
		return func(context.Context) func(string) bool { return skip }
	}
	tests := []struct {
		name     string
		suppress CertSuppressor
		want     int
	}{
		{"nothing suppressed", nil, 2},
		{"proxy-managed host hidden", only(func(d string) bool { return d == "app.example.com" }), 1},
		{"upstream hides all", only(func(string) bool { return true }), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := NewSuppressedCertSource(src, tc.suppress)
			_, notices, err := EvaluateCertExpiring(context.Background(), wrapped, Rule{ID: "r", Kind: KindCertExpiring, Threshold: 14}, now, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(notices) != tc.want {
				t.Fatalf("cert_expiring notices = %v, want %d", notices, tc.want)
			}
			next, notices, err := EvaluateCertExpiry(context.Background(), wrapped, newFakeCertExpiryObservationStore(),
				Rule{ID: "e", Kind: KindCertExpiry}, 14*24*time.Hour, DefaultCertRenewalStalledThreshold, now, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(notices) != tc.want || (tc.want == 0 && next.Firing) {
				t.Fatalf("cert_expiry notices = %v firing=%v, want %d", notices, next.Firing, tc.want)
			}
		})
	}
}

type fixedACME struct{}

func (fixedACME) ACMEFailing(string) (string, bool) { return "tls: unrecognized name", true }

func TestSuppressedACMESource(t *testing.T) {
	skip := func(context.Context) func(string) bool {
		return func(d string) bool { return d == "app.example.com" }
	}
	src := NewSuppressedACMESource(fixedACME{}, skip)
	if _, failing := src.ACMEFailing("app.example.com"); failing {
		t.Fatal("proxy-managed host must not report an ACME failure")
	}
	if _, failing := src.ACMEFailing("other.example.com"); !failing {
		t.Fatal("other host must keep its failure")
	}
}

func TestStorageKeyDomain(t *testing.T) {
	tests := map[string]string{
		"certificates/acme-v02/a.example.com/a.example.com.crt":                 "a.example.com",
		"certificates/acme-v02/wildcard_.example.com/wildcard_.example.com.crt": "*.example.com",
	}
	for key, want := range tests {
		if got := storageKeyDomain(key); got != want {
			t.Errorf("storageKeyDomain(%q) = %q, want %q", key, got, want)
		}
	}
}
