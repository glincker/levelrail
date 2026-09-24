package alerting

import (
	"context"
	"testing"
	"time"
)

func TestCertRenewalStates(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	notAfter := now.Add(5 * 24 * time.Hour)
	infos := []CertInfo{
		{Domain: "ok.example.com", Status: "healthy", NotAfter: now.Add(60 * 24 * time.Hour)},
		{Domain: "expired.example.com", Status: "expired", NotAfter: now.Add(-time.Hour)},
		{Domain: "fresh.example.com", Status: "expiring_soon", NotAfter: notAfter},
		{Domain: "stuck.example.com", Status: "expiring_soon", NotAfter: notAfter},
		{Domain: "renewed.example.com", Status: "expiring_soon", NotAfter: notAfter},
	}
	obs := []CertExpiryObservation{
		{Domain: "fresh.example.com", Status: "expiring_soon", EpisodeNotAfter: notAfter, EpisodeStartedAt: now.Add(-time.Hour)},
		{Domain: "stuck.example.com", Status: "expiring_soon", EpisodeNotAfter: notAfter, EpisodeStartedAt: now.Add(-7 * time.Hour)},
		{Domain: "renewed.example.com", Status: "expiring_soon", EpisodeNotAfter: notAfter.Add(-90 * 24 * time.Hour), EpisodeStartedAt: now.Add(-7 * time.Hour)},
	}

	got := CertRenewalStates(infos, obs, 0, now)
	want := map[string]string{
		"ok.example.com":      CertRenewalOK,
		"expired.example.com": CertRenewalStalled,
		"fresh.example.com":   CertRenewalOK,
		"stuck.example.com":   CertRenewalStalled,
		"renewed.example.com": CertRenewalOK,
	}
	for domain, w := range want {
		if got[domain] != w {
			t.Errorf("%s: renewal = %q, want %q", domain, got[domain], w)
		}
	}
}

func TestListAllCertExpiryObservations(t *testing.T) {
	ctx := context.Background()
	db := newTestDeployNotifyDB(t)
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)

	for _, id := range []string{"r1", "r2"} {
		if err := db.SaveRule(ctx, Rule{ID: id, Name: id, Kind: KindCertExpiry, Enabled: true}); err != nil {
			t.Fatalf("SaveRule(%s) error = %v", id, err)
		}
		o := CertExpiryObservation{
			RuleID: id, Domain: id + ".example.com", Status: "expiring_soon",
			NotAfter: now, EpisodeNotAfter: now, EpisodeStartedAt: now, ObservedAt: now,
		}
		if err := db.UpsertCertExpiryObservation(ctx, o); err != nil {
			t.Fatalf("Upsert(%s) error = %v", id, err)
		}
	}

	got, err := db.ListAllCertExpiryObservations(ctx)
	if err != nil {
		t.Fatalf("ListAllCertExpiryObservations() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d observations, want 2: %+v", len(got), got)
	}
}
