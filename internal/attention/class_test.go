package attention

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestClassOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		kind, subject, want string
	}{
		{"deploy", "web", ClassIncident},
		{"app", "web", ClassIncident},
		{"node", "n1", ClassIncident},
		{"disk", "data dir", ClassIncident},
		{KindBackupOverdue, "db", ClassIncident},
		{KindLoginAnomaly, "1.2.3.4", ClassIncident},
		{KindDockerGuard, "would-deny", ClassRecommendation},
		{KindDockerGuard, "ready", ClassRecommendation},
		{KindDockerGuard, "denied", ClassIncident},
		{KindDBEOL, "pg", ClassRecommendation},
		{KindDBSecurityUpdates, "", ClassRecommendation},
		{KindInvites, "", ClassRecommendation},
		{"update", "control plane", ClassRecommendation},
		{"node_agent", "n1", ClassRecommendation},
	}
	for _, tc := range tests {
		if got := ClassOf(tc.kind, tc.subject); got != tc.want {
			t.Errorf("ClassOf(%q, %q) = %q, want %q", tc.kind, tc.subject, got, tc.want)
		}
	}
}

func TestDoctorClass(t *testing.T) {
	t.Parallel()
	tests := []struct {
		code, status, want string
	}{
		{"control_plane_dr", "warn", ClassRecommendation},
		{"docker_guard", "warn", ClassRecommendation},
		{"firewall", "warn", ClassRecommendation},
		{"disk_space", "warn", ClassIncident},
		{"database", "fail", ClassIncident},
		{"firewall", "fail", ClassIncident},
		{"port_443", "warn", ClassIncident},
	}
	for _, tc := range tests {
		if got := DoctorClass(tc.code, tc.status); got != tc.want {
			t.Errorf("DoctorClass(%q, %q) = %q, want %q", tc.code, tc.status, got, tc.want)
		}
	}
}

func TestCertExpiryClass(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	const shortLived = 7 * 24 * time.Hour
	tests := []struct {
		name string
		in   CertFacts
		want string
	}{
		{"internal CA leaf expiring in 3 hours", CertFacts{
			Issuer: "Caddy Local Authority - ECC Intermediate", Source: "acme", TLSOwner: "acme",
			Status: "expiring_soon", NotBefore: now.Add(-9 * time.Hour), NotAfter: now.Add(3 * time.Hour), Routed: true,
		}, ""},
		{"internal owner, stale and expired", CertFacts{
			Source: "acme", TLSOwner: "internal", Status: "expired", Routed: true,
			NotBefore: now.Add(-100 * 24 * time.Hour), NotAfter: now.Add(-time.Hour),
		}, ""},
		{"proxy managed host", CertFacts{
			Source: "proxy", TLSOwner: "proxy", Status: "expired", Routed: true,
			NotBefore: now.Add(-80 * 24 * time.Hour), NotAfter: now.Add(-time.Hour),
		}, ""},
		{"orphaned certificate", CertFacts{
			Issuer: "R11", Source: "acme", TLSOwner: "acme", Status: "expired", Routed: false,
			NotBefore: now.Add(-90 * 24 * time.Hour), NotAfter: now.Add(-time.Hour),
		}, ""},
		{"real ACME in 90 days", CertFacts{
			Issuer: "R11", Source: "acme", TLSOwner: "acme", Status: "healthy", Routed: true,
			NotBefore: now, NotAfter: now.Add(90 * 24 * time.Hour),
		}, ""},
		{"real ACME inside window, renewal ok", CertFacts{
			Issuer: "R11", Source: "acme", TLSOwner: "acme", Status: "expiring_soon", Renewal: "ok", Routed: true,
			NotBefore: now.Add(-80 * 24 * time.Hour), NotAfter: now.Add(10 * 24 * time.Hour),
		}, ""},
		{"real ACME in 5 days, stalled", CertFacts{
			Issuer: "R11", Source: "acme", TLSOwner: "acme", Status: "expiring_soon", Renewal: "stalled", Routed: true,
			NotBefore: now.Add(-85 * 24 * time.Hour), NotAfter: now.Add(5 * 24 * time.Hour),
		}, ClassIncident},
		{"real ACME expired", CertFacts{
			Issuer: "R11", Source: "acme", TLSOwner: "acme", Status: "expired", Routed: true,
			NotBefore: now.Add(-91 * 24 * time.Hour), NotAfter: now.Add(-time.Hour),
		}, ClassIncident},
		{"real ACME healthy but last renewal failed", CertFacts{
			Issuer: "R11", Source: "acme", TLSOwner: "acme", Status: "healthy", Routed: true, RenewalFailed: true,
			NotBefore: now.Add(-60 * 24 * time.Hour), NotAfter: now.Add(30 * 24 * time.Hour),
		}, ClassIncident},
		{"real ACME inside the failure window", CertFacts{
			Issuer: "R11", Source: "acme", TLSOwner: "acme", Status: "expiring_soon", Routed: true, NearExpiry: true,
			NotBefore: now.Add(-80 * 24 * time.Hour), NotAfter: now.Add(10 * 24 * time.Hour),
		}, ClassIncident},
		{"internal CA with a failed flag still never alerts", CertFacts{
			Issuer: "Caddy Local Authority - ECC Intermediate", Source: "acme", TLSOwner: "internal", Status: "healthy", Routed: true, RenewalFailed: true,
			NotBefore: now, NotAfter: now.Add(12 * time.Hour),
		}, ""},
		{"uploaded certificate in window", CertFacts{
			Issuer: "Corp CA", Source: "custom", TLSOwner: "custom", Status: "expiring_soon", Routed: true,
			NotBefore: now.Add(-300 * 24 * time.Hour), NotAfter: now.Add(20 * 24 * time.Hour),
		}, ClassIncident},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := CertExpiryClass(tc.in, shortLived); got != tc.want {
				t.Errorf("CertExpiryClass = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuild_CertsNeedIncidentClassAndDoctorWarnIsRecommendation(t *testing.T) {
	t.Parallel()
	in := Input{
		Certs: []apiclient.CertificateResource{
			{Domain: "stale.example", Status: "expired", NotAfter: time.Unix(0, 0)},
			{Domain: "real.example", Status: "expired", ExpiryClass: ClassIncident, NotAfter: time.Unix(0, 0)},
		},
		Doctor: apiclient.SystemDoctorResource{Checks: []apiclient.DoctorCheckResource{
			{Code: "control_plane_dr", Name: "DR", Status: "warn"},
		}},
	}
	var certs, recs int
	for _, it := range Build(in) {
		if it.Kind == "certificate" {
			certs++
			if it.Subject != "real.example" {
				t.Errorf("unexpected certificate item %q", it.Subject)
			}
		}
		if it.Class == ClassRecommendation {
			recs++
		}
	}
	if certs != 1 || recs != 1 {
		t.Errorf("certs=%d recs=%d, want 1 and 1", certs, recs)
	}
}
