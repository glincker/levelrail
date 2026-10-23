package migrateverify

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport"
)

func find(diffs []Difference, cat Category, subject string) (Difference, bool) {
	for _, d := range diffs {
		if d.Category == cat && d.Subject == subject {
			return d, true
		}
	}
	return Difference{}, false
}

func TestCompareClassification(t *testing.T) {
	baseSrc := SourceApp{ID: "1", Name: "Web", Domains: []string{"Web.Example.com."}, EnvKeys: []string{"DB_URL", "API_KEY", "NEXT_PUBLIC_X"},
		BuildTimeKeys: []string{"NEXT_PUBLIC_X"}, Volumes: []string{"/data"}, Jobs: []Job{{Name: "Nightly Backup", Schedule: "@daily", Enabled: true}}}
	baseTgt := TargetApp{Name: "web", SourceKey: "coolify:1", Domains: []string{"web.example.com"}, EnvKeys: []string{"DB_URL", "API_KEY", "NEXT_PUBLIC_X"},
		Volumes: []string{"/data"}, Jobs: []Job{{Name: "nightly-backup", Schedule: "0 0 * * *", Enabled: true}}}

	tests := []struct {
		name    string
		mutate  func(s *SourceApp, tg *TargetApp)
		cat     Category
		subject string
		want    Severity // empty means no difference expected
	}{
		{"identical apps have no differences", func(*SourceApp, *TargetApp) {}, CatApps, "web", ""},
		{"domain missing", func(_ *SourceApp, tg *TargetApp) { tg.Domains = nil }, CatDomains, "web.example.com", Blocking},
		{"domain held back while staged", func(_ *SourceApp, tg *TargetApp) { tg.Domains, tg.Staged = nil, true }, CatDomains, "web.example.com", Warning},
		{"extra domain", func(_ *SourceApp, tg *TargetApp) { tg.Domains = append(tg.Domains, "new.example.com") }, CatDomains, "new.example.com", Info},
		{"env name missing", func(_ *SourceApp, tg *TargetApp) { tg.EnvKeys = []string{"API_KEY", "NEXT_PUBLIC_X"} }, CatEnv, "DB_URL", Blocking},
		{"build time env name missing", func(_ *SourceApp, tg *TargetApp) { tg.EnvKeys = []string{"DB_URL", "API_KEY"} }, CatEnv, "NEXT_PUBLIC_X", Warning},
		{"extra env name", func(_ *SourceApp, tg *TargetApp) { tg.EnvKeys = append(tg.EnvKeys, "EXTRA") }, CatEnv, "EXTRA", Info},
		{"volume missing", func(_ *SourceApp, tg *TargetApp) { tg.Volumes = nil }, CatVolumes, "/data", Blocking},
		{"extra volume", func(_ *SourceApp, tg *TargetApp) { tg.Volumes = append(tg.Volumes, "/cache") }, CatVolumes, "/cache", Info},
		{"job missing", func(_ *SourceApp, tg *TargetApp) { tg.Jobs = nil }, CatJobs, "Nightly Backup", Warning},
		{"job schedule differs", func(_ *SourceApp, tg *TargetApp) { tg.Jobs[0].Schedule = "5 1 * * *" }, CatJobs, "Nightly Backup", Warning},
		{"job disabled", func(_ *SourceApp, tg *TargetApp) { tg.Jobs[0].Enabled = false }, CatJobs, "Nightly Backup", Warning},
		{"job disabled while staged", func(_ *SourceApp, tg *TargetApp) { tg.Jobs[0].Enabled, tg.Staged = false, true }, CatJobs, "Nightly Backup", Info},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, tg := baseSrc, baseTgt
			s.Jobs = append([]Job(nil), baseSrc.Jobs...)
			tg.Jobs = append([]Job(nil), baseTgt.Jobs...)
			tg.Domains = append([]string(nil), baseTgt.Domains...)
			tg.EnvKeys = append([]string(nil), baseTgt.EnvKeys...)
			tg.Volumes = append([]string(nil), baseTgt.Volumes...)
			tc.mutate(&s, &tg)
			got := Compare(Source{Platform: "coolify", Apps: []SourceApp{s}}, Target{Apps: []TargetApp{tg}})
			d, ok := find(got.Differences, tc.cat, tc.subject)
			if tc.want == "" {
				if len(got.Differences) != 0 {
					t.Fatalf("want no differences, got %+v", got.Differences)
				}
				return
			}
			if !ok || d.Severity != tc.want {
				t.Fatalf("want %s on %s %q, got %+v", tc.want, tc.cat, tc.subject, got.Differences)
			}
			if d.Fix == "" || d.Detail == "" {
				t.Fatalf("difference needs a detail and a fix hint: %+v", d)
			}
		})
	}
}

func TestCompareAppsAndDatabases(t *testing.T) {
	src := Source{Platform: "dokploy",
		Apps: []SourceApp{{ID: "a", Name: "Blog"}, {ID: "b", Name: "Shop"}},
		DBs:  []DB{{Name: "main", Engine: "postgres", Version: "16"}, {Name: "cache", Engine: "redis", Version: "7"}, {Name: "docs", Engine: "mysql", Version: "8"}, {Name: "gone", Engine: "postgres"}}}
	tgt := Target{
		Apps: []TargetApp{{Name: "renamed-blog", SourceKey: "dokploy:a"}, {Name: "stray"}},
		DBs:  []DB{{Name: "main", Engine: "postgres", Version: "15"}, {Name: "cache", Engine: "valkey", Version: "7"}, {Name: "docs", Engine: "mysql", Version: "8"}, {Name: "extra", Engine: "redis"}}}
	got := Compare(src, tgt)

	want := []struct {
		cat Category
		sub string
		sev Severity
	}{
		{CatApps, "Shop", Blocking},
		{CatApps, "stray", Info},
		{CatDatabases, "main", Warning},
		{CatDatabases, "cache", Blocking},
		{CatDatabases, "gone", Blocking},
		{CatDatabases, "extra", Info},
	}
	for _, w := range want {
		if d, ok := find(got.Differences, w.cat, w.sub); !ok || d.Severity != w.sev {
			t.Errorf("want %s on %s %q, got %+v", w.sev, w.cat, w.sub, got.Differences)
		}
	}
	if _, ok := find(got.Differences, CatDatabases, "docs"); ok {
		t.Error("an identical database must not differ")
	}
	if _, ok := find(got.Differences, CatApps, "Blog"); ok {
		t.Error("an app matched by its import label must not differ")
	}
}

func TestCompareProbesClassification(t *testing.T) {
	ok := func(status int, mut func(*ProbeResult)) ProbeResult {
		r := ProbeResult{URL: "https://x/", Status: status, TLS: true, TLSValid: true, TLSDaysLeft: 80, LatencyMS: 100}
		if mut != nil {
			mut(&r)
		}
		return r
	}
	th := Thresholds{TLSWarnDays: 14, LatencyFactor: 3, LatencyFloorMS: 300}
	tests := []struct {
		name   string
		probe  DomainProbe
		want   Severity
		detail string
	}{
		{"equal", DomainProbe{Source: ok(200, nil), Target: ok(200, nil)}, "", ""},
		{"target error", DomainProbe{Source: ok(200, nil), Target: ProbeResult{Error: "connection refused"}}, Blocking, "did not answer"},
		{"both error", DomainProbe{Source: ProbeResult{Error: "x"}, Target: ProbeResult{Error: "y"}}, Warning, "neither"},
		{"source error", DomainProbe{Source: ProbeResult{Error: "x"}, Target: ok(200, nil)}, Info, "source did not"},
		{"target 500", DomainProbe{Source: ok(200, nil), Target: ok(502, nil)}, Blocking, "status differs"},
		{"ok to 404", DomainProbe{Source: ok(200, nil), Target: ok(404, nil)}, Blocking, "status differs"},
		{"200 to 204", DomainProbe{Source: ok(200, nil), Target: ok(204, nil)}, Warning, "status differs"},
		{"cert invalid", DomainProbe{Source: ok(200, nil), Target: ok(200, func(r *ProbeResult) { r.TLSValid, r.TLSError = false, "x509" })}, Blocking, "certificate is not valid"},
		{"cert invalid while staged", DomainProbe{Staged: true, Source: ok(200, nil), Target: ok(200, func(r *ProbeResult) { r.TLSValid, r.TLSError = false, "x509" })}, Warning, "certificate is not valid"},
		{"cert near expiry", DomainProbe{Source: ok(200, nil), Target: ok(200, func(r *ProbeResult) { r.TLSDaysLeft = 5 })}, Warning, "expires in 5 days"},
		{"redirect differs", DomainProbe{Source: ok(301, func(r *ProbeResult) { r.RedirectTarget = "https://a/" }), Target: ok(301, func(r *ProbeResult) { r.RedirectTarget = "https://b/" })}, Warning, "redirect differs"},
		{"slower", DomainProbe{Source: ok(200, nil), Target: ok(200, func(r *ProbeResult) { r.LatencyMS = 900 })}, Warning, "slower"},
		{"slow but under floor", DomainProbe{Source: ok(200, func(r *ProbeResult) { r.LatencyMS = 10 }), Target: ok(200, func(r *ProbeResult) { r.LatencyMS = 250 })}, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.probe.Domain = "x.example.com"
			got := CompareProbes([]DomainProbe{tc.probe}, th)
			if tc.want == "" {
				if len(got) != 0 {
					t.Fatalf("want none, got %+v", got)
				}
				return
			}
			if len(got) != 1 || got[0].Severity != tc.want || !strings.Contains(got[0].Detail, tc.detail) || got[0].Fix == "" {
				t.Fatalf("want one %s containing %q, got %+v", tc.want, tc.detail, got)
			}
		})
	}
}

func TestThresholdsFromEnv(t *testing.T) {
	env := map[string]string{EnvTLSWarnDays: "30", EnvLatencyFactor: "2.5", EnvLatencyFloorMS: "50"}
	th := ThresholdsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if th.TLSWarnDays != 30 || th.LatencyFactor != 2.5 || th.LatencyFloorMS != 50 {
		t.Fatalf("env not applied: %+v", th)
	}
	bad := ThresholdsFromEnv(func(string) (string, bool) { return "nope", true })
	if bad.TLSWarnDays != defaultTLSWarnDays {
		t.Fatalf("invalid env must fall back: %+v", bad)
	}
}

func TestFinishVerdictAndOrder(t *testing.T) {
	r := Report{Categories: []Totals{{Category: CatEnv}}, Differences: []Difference{
		{Category: CatEnv, Severity: Info, Subject: "a"}, {Category: CatEnv, Severity: Blocking, Subject: "b"}, {Category: CatEnv, Severity: Warning, Subject: "c"}}}
	r.Finish()
	if r.Verdict != VerdictFail || r.Counts != (Counts{1, 1, 1}) || r.Differences[0].Severity != Blocking || !r.HasBlocking() {
		t.Fatalf("unexpected report: %+v", r)
	}
	if r.Categories[0].Blocking != 1 || r.Categories[0].Info != 1 {
		t.Fatalf("category counts: %+v", r.Categories[0])
	}
	pass := Report{}
	pass.Finish()
	if pass.Verdict != VerdictPass {
		t.Fatalf("empty report must pass: %s", pass.Verdict)
	}
}

func TestNoEnvValueEverAppearsInTheReport(t *testing.T) {
	const secret = "s3cr3t-value-9f8e7d"
	disc := &platformimport.Discovery{Platform: platformimport.Coolify, Apps: []platformimport.App{{
		SourceID: "1", Name: "Web", Domains: []string{"web.example.com"},
		Env: []platformimport.Env{{Key: "DB_URL", Value: "postgres://u:" + secret + "@db/x", Secret: true}, {Key: "TOKEN", Value: secret}, {Key: "BUILD", Value: secret, BuildTime: true}},
	}}}
	src := SnapshotSource(disc)
	tgt := Target{Apps: []TargetApp{{Name: "web", SourceKey: "coolify:1", Domains: []string{"web.example.com"}, EnvKeys: []string{"TOKEN"}}}}
	rep := Build(context.Background(), src, tgt, nil, Thresholds{}, time.Unix(0, 0))

	out, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), secret) {
		t.Fatalf("an env value leaked into the report: %s", out)
	}
	if _, ok := find(rep.Differences, CatEnv, "DB_URL"); !ok {
		t.Fatalf("the env NAME should be reported: %s", out)
	}
	snap, _ := json.Marshal(src)
	if strings.Contains(string(snap), secret) {
		t.Fatal("snapshot holds a value")
	}
}

func TestBuildProbesEveryMatchedDomain(t *testing.T) {
	src := Source{Platform: "coolify", Apps: []SourceApp{{ID: "1", Name: "Web", Domains: []string{"web.example.com", "*.example.com"}}}}
	tgt := Target{Apps: []TargetApp{{Name: "web", SourceKey: "coolify:1", Domains: []string{"web.example.com"}}}}
	rep := Build(context.Background(), src, tgt, &Prober{Opts: ProbeOptions{Timeout: time.Second, Concurrency: 1, SourceAddr: "127.0.0.1:1", TargetAddr: "127.0.0.1:1"}}, Thresholds{TLSWarnDays: 14, LatencyFactor: 3}, time.Now())
	if len(rep.Probes) != 1 {
		t.Fatalf("want 1 probe (wildcard skipped), got %d", len(rep.Probes))
	}
	if _, ok := find(rep.Differences, CatProbes, "*.example.com"); !ok {
		t.Fatal("a wildcard domain should be listed as not probed")
	}
	for _, p := range rep.Probes {
		if !strings.Contains(p.Target.Error, "not allowed") {
			t.Fatalf("loopback must be refused without the flag: %+v", p.Target)
		}
	}
	if rep.Verdict != VerdictFail {
		t.Fatalf("a target that cannot be probed is blocking, got %s", rep.Verdict)
	}
}
