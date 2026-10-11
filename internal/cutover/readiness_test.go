package cutover

import "testing"

func goodFacts() Facts {
	return Facts{
		App: "shop", ItemState: ItemVerified,
		ImageSource: ImageRegistry, Image: "nginx:1",
		EnvPlain: 3, EnvSecret: 1,
		HealthKnown: true, HealthPath: "/healthz",
		Domains: []DomainFact{{Domain: "shop.example.com", Current: []string{"A 203.0.113.9"},
			Plan: DomainPlan{Method: MethodDNS, Provider: "cloudflare", Desired: &Record{Name: "shop", Type: "A", Value: "198.51.100.4"}}}},
	}
}

func checkByID(p Plan, id string) []Check {
	var out []Check
	for _, c := range p.Checks {
		if c.ID == id {
			out = append(out, c)
		}
	}
	return out
}

func TestEvaluateRules(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Facts)
		id      string
		status  string
		verdict string
		fixStep string
	}{
		{"all good", func(*Facts) {}, CheckImage, StatusPass, VerdictReady, ""},
		{"not verified", func(f *Facts) { f.ItemState = "staged" }, CheckState, StatusBlock, VerdictBlocked, StepLinkVerify},
		{"host built missing", func(f *Facts) { f.ImageSource = ImageHostBuilt }, CheckImage, StatusBlock, VerdictBlocked, StepLinkImages},
		{"host built verified", func(f *Facts) { f.ImageSource = ImageHostBuilt; f.ImageVerified = true }, CheckImage, StatusPass, VerdictReady, ""},
		{"git built", func(f *Facts) { f.ImageSource = ImageGit }, CheckImage, StatusPass, VerdictReady, ""},
		{"empty secrets", func(f *Facts) { f.EnvEmptySecrets = 2 }, CheckEnv, StatusBlock, VerdictBlocked, "app-env"},
		{"database not moved", func(f *Facts) { f.Databases = []DBFact{{Name: "pg"}} }, CheckDatabase, StatusBlock, VerdictBlocked, "database-hub"},
		{"database external", func(f *Facts) { f.Databases = []DBFact{{Name: "pg", External: true, Host: "db.rds"}} }, CheckDatabase, StatusPass, VerdictReady, ""},
		{"database mismatch", func(f *Facts) {
			f.Databases = []DBFact{{Name: "pg", Target: "pg", Verified: true, Checked: 10, Mismatched: 2}}
		}, CheckDatabase, StatusBlock, VerdictBlocked, "database-hub"},
		{"database matches", func(f *Facts) {
			f.Databases = []DBFact{{Name: "pg", Target: "pg", Verified: true, Checked: 10}}
		}, CheckDatabase, StatusPass, VerdictReady, ""},
		{"database no row count", func(f *Facts) { f.Databases = []DBFact{{Name: "pg", Target: "pg"}} }, CheckDatabase, StatusWarn, VerdictWarn, "database-hub"},
		{"volume pending", func(f *Facts) { f.Volumes = []VolumeFact{{Name: "data"}} }, CheckVolumes, StatusWarn, VerdictWarn, StepLinkVolumes},
		{"volume copied", func(f *Facts) { f.Volumes = []VolumeFact{{Name: "data", Copied: true}} }, CheckVolumes, StatusPass, VerdictReady, ""},
		{"no health", func(f *Facts) { f.HealthKnown = false; f.HealthPath = "" }, CheckHealth, StatusWarn, VerdictWarn, "app-settings"},
		{"no domains", func(f *Facts) { f.Domains = nil }, CheckDomains, StatusBlock, VerdictBlocked, "app-domains"},
		{"domain taken", func(f *Facts) { f.Domains[0].TakenBy = "other" }, CheckDomains, StatusBlock, VerdictBlocked, "app-domains"},
		{"already here", func(f *Facts) { f.Domains[0].PointsHere = true }, CheckDomains, StatusPass, VerdictReady, ""},
		{"manual record", func(f *Facts) { f.Domains[0].Plan.Method = MethodManual; f.Domains[0].Plan.Provider = "" }, CheckDomains, StatusWarn, VerdictWarn, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := goodFacts()
			tc.mutate(&f)
			p := Evaluate(f)
			cs := checkByID(p, tc.id)
			if len(cs) == 0 {
				t.Fatalf("no %s check in %+v", tc.id, p.Checks)
			}
			c := cs[0]
			if c.Status != tc.status {
				t.Fatalf("%s status = %s (%s), want %s", tc.id, c.Status, c.Detail, tc.status)
			}
			if p.Verdict != tc.verdict {
				t.Fatalf("verdict = %s, want %s", p.Verdict, tc.verdict)
			}
			if tc.fixStep != "" {
				if c.Fix == nil || c.Fix.Action == nil || c.Fix.Action.Value != tc.fixStep {
					t.Fatalf("fix = %+v, want link to %q", c.Fix, tc.fixStep)
				}
			}
			if c.Status != StatusPass && c.Fix == nil {
				t.Fatalf("a %s check must carry a fix", c.Status)
			}
		})
	}
}

func TestEvaluateDomainPlans(t *testing.T) {
	f := goodFacts()
	f.Domains[0].PointsHere = true
	p := Evaluate(f)
	if got := p.Domains[0].Method; got != MethodNone {
		t.Fatalf("method = %s, want none", got)
	}
	if p.HealthPath != "/healthz" {
		t.Fatalf("health path = %q", p.HealthPath)
	}
	f = goodFacts()
	f.HealthPath = ""
	if got := Evaluate(f).HealthPath; got != "/" {
		t.Fatalf("default health path = %q", got)
	}
}
