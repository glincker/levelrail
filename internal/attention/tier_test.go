package attention

import "testing"

func TestAssessTierTable(t *testing.T) {
	cases := []struct {
		name string
		sig  Signal
		want string
	}{
		{"doctor fail", Signal{Kind: "docker", Status: "fail", Class: ClassIncident}, TierCritical},
		{"doctor warn incident", Signal{Kind: "disk_space", Status: "warn", Class: ClassIncident}, TierNeedsAction},
		{"doctor warn recommendation", Signal{Kind: "ingress_survives", Status: "warn", Class: ClassRecommendation}, TierAdvisory},
		{"doctor ok has no tier", Signal{Kind: "docker", Status: "ok"}, ""},
		{"low exposure is advisory", Signal{Kind: "exposure_n_tcp_80", Status: "warn", Class: ClassIncident, Message: "[low] port 80"}, TierAdvisory},
		{"feed critical", Signal{Kind: "node", Severity: Critical}, TierCritical},
		{"feed warning incident", Signal{Kind: "token_expiring", Severity: Warning, Class: ClassIncident}, TierNeedsAction},
		{"feed warning empty class", Signal{Kind: "certificate", Severity: Warning}, TierNeedsAction},
		{"feed warning recommendation", Signal{Kind: KindDBSecurityUpdates, Severity: Warning, Class: ClassRecommendation}, TierAdvisory},
		{"feed info", Signal{Kind: KindResourceAdvice, Severity: Info}, TierAdvisory},
		{"handled login", Signal{Kind: KindDeviceLoginResolved, Severity: Info}, TierAutoResolved},
	}
	for _, c := range cases {
		if got, _ := Assess(c.sig); got != c.want {
			t.Errorf("%s: tier %q, want %q", c.name, got, c.want)
		}
	}
}

func TestAssessGroup(t *testing.T) {
	cases := []struct {
		kind, subject, want string
	}{
		{"docker_guard", "", GroupDockerAccess},
		{"docker_privilege", "", GroupDockerAccess},
		{"disk_space", "", GroupDisk},
		{"disk", "", GroupDisk},
		{"certificates", "", GroupCertificates},
		{"node_agent_n1", "", GroupNodes},
		{"node", "web-1", GroupNodes},
		{"token_expired", "", GroupTokens},
		{"backup_unprotected", "", GroupBackups},
		{"control_plane_dr", "", GroupBackups},
		{"exposure_n_tcp_80", "coolify-proxy", "exposure:coolify-proxy"},
		{"exposure_n_tcp_80", "", GroupExposure},
		{"clock_skew", "", ""},
	}
	for _, c := range cases {
		if _, got := Assess(Signal{Kind: c.kind, Subject: c.subject, Status: "warn"}); got != c.want {
			t.Errorf("group of %s/%s = %q, want %q", c.kind, c.subject, got, c.want)
		}
	}
}

func TestHeaderCountOnlyCriticalAndNeedsAction(t *testing.T) {
	items := []Item{
		{Tier: TierCritical}, {Tier: TierNeedsAction}, {Tier: TierAdvisory}, {Tier: TierAutoResolved}, {},
	}
	if got := HeaderCount(items); got != 2 {
		t.Fatalf("HeaderCount = %d, want 2", got)
	}
}

func TestBuildStampsTierAndGroup(t *testing.T) {
	for _, it := range Build(Input{Feed: nil}) {
		if it.Tier == "" {
			t.Errorf("item %s has no tier", it.ID)
		}
	}
}
