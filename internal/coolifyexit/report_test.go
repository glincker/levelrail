package coolifyexit_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/coolifyexit"
	"github.com/GLINCKER/levelrail/internal/coolifyexit/exportfixture"
	"github.com/GLINCKER/levelrail/internal/platformimport"
)

func discover(t *testing.T) (*exportfixture.Server, *platformimport.Discovery) {
	t.Helper()
	srv := exportfixture.New(t)
	src, err := platformimport.NewCoolify(srv.URL, exportfixture.Token, platformimport.ClientOptions{Policy: platformimport.NetworkPolicy{AllowLoopback: true}})
	if err != nil {
		t.Fatal(err)
	}
	d, err := src.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return srv, d
}

func rowsOf(r coolifyexit.Report, kind string) []coolifyexit.Row {
	var out []coolifyexit.Row
	for _, row := range r.Rows {
		if row.Kind == kind {
			out = append(out, row)
		}
	}
	return out
}

func find(t *testing.T, r coolifyexit.Report, kind, name string) coolifyexit.Row {
	t.Helper()
	for _, row := range r.Rows {
		if row.Kind == kind && row.Name == name {
			return row
		}
	}
	t.Fatalf("no %s row named %q", kind, name)
	return coolifyexit.Row{}
}

func TestExitReportDryRun(t *testing.T) {
	srv, d := discover(t)
	if v := srv.Violations(); len(v) != 0 {
		t.Fatalf("source was written to: %v", v)
	}
	plan := platformimport.BuildPlan(d, platformimport.PlanOptions{})
	r := coolifyexit.Build(d, plan)

	if r.Mode != coolifyexit.ModeDryRun || r.ReadyToRetire {
		t.Fatalf("mode=%s ready=%v", r.Mode, r.ReadyToRetire)
	}

	// Engines the managed catalog lacks must be reported, not dropped.
	for _, name := range []string{"sessions", "analytics"} {
		if row := find(t, r, "database", name); row.Outcome != coolifyexit.Skipped {
			t.Errorf("%s outcome = %s, want skipped", name, row.Outcome)
		}
	}
	if row := find(t, r, "app", "collab"); row.Outcome != coolifyexit.Skipped {
		t.Errorf("compose app outcome = %s", row.Outcome)
	}
	if row := find(t, r, "service", "plausible"); row.Outcome != coolifyexit.Skipped {
		t.Errorf("one-click service outcome = %s", row.Outcome)
	}

	web := find(t, r, "app", "storefront")
	joined := strings.Join(web.Reasons, "\n")
	for _, want := range []string{"custom build command", "custom start command", "custom container labels", "build-time only variables", "preview deployments", "www redirect"} {
		if !strings.Contains(joined, want) {
			t.Errorf("storefront reasons lack %q:\n%s", want, joined)
		}
	}
	if web.Outcome != coolifyexit.Manual {
		t.Errorf("storefront outcome = %s", web.Outcome)
	}
	if !strings.Contains(strings.Join(find(t, r, "app", "docs site").Reasons, "\n"), "basic auth") {
		t.Error("basic auth on a source app must be called out")
	}

	if got := len(rowsOf(r, "cron")); got != 2 {
		t.Errorf("cron rows = %d, want the 2 enabled tasks", got)
	}
	if got := rowsOf(r, "webhook"); len(got) != 1 || got[0].Outcome != coolifyexit.Manual {
		t.Errorf("webhook rows = %+v", got)
	}
	vol := rowsOf(r, "volume")
	if len(vol) != 1 || !strings.Contains(strings.Join(vol[0].Reasons, " "), "2.0 GiB") {
		t.Errorf("volume rows = %+v", vol)
	}
	if len(rowsOf(r, "domain")) < 5 {
		t.Errorf("domain rows = %d", len(rowsOf(r, "domain")))
	}

	data := find(t, r, "database-data", "main-db")
	if len(data.Next) != 1 || !strings.Contains(data.Next[0], "--port 54320") || !strings.Contains(data.Next[0], "--user app") || !strings.Contains(data.Next[0], "--database shop") {
		t.Errorf("db-copy hint = %v", data.Next)
	}
	if bk := rowsOf(r, "backup"); len(bk) != 1 || !strings.Contains(bk[0].Reasons[0], "S3") {
		t.Errorf("backup rows = %+v", bk)
	}
	if find(t, r, "server", "hetzner-1").Outcome != coolifyexit.Manual || find(t, r, "server", "builder").Outcome != coolifyexit.Imported {
		t.Error("proxy owning ports must be manual, build server must not")
	}

	assertWaves(t, r)
	assertChecklist(t, r)
	assertNoSecrets(t, r)
}

func assertWaves(t *testing.T, r coolifyexit.Report) {
	t.Helper()
	byApp := map[string]int{}
	for _, w := range r.Waves {
		for _, a := range w.Apps {
			byApp[a.Name] = w.Number
		}
	}
	if byApp["umami"] != 1 || byApp["go-api"] != 1 || byApp["docs-site"] != 1 {
		t.Errorf("stateless apps must switch first: %v", byApp)
	}
	if byApp["storefront"] != 3 {
		t.Errorf("an app with a volume and a database switches last: %v", byApp)
	}
	if _, ok := byApp["collab"]; ok {
		t.Error("a skipped app must not be scheduled for DNS")
	}
}

func assertChecklist(t *testing.T, r coolifyexit.Report) {
	t.Helper()
	var ids []string
	for _, s := range r.Checklist {
		ids = append(ids, s.ID)
	}
	order := []string{"backup-source", "resolve-manual", "stage", "copy-data", "verify-staged", "lower-ttl", "recreate-hooks", "switch-wave-1", "switch-wave-3", "keep-fallback", "retire"}
	pos := -1
	for _, want := range order {
		found := -1
		for i, id := range ids {
			if id == want {
				found = i
			}
		}
		if found <= pos {
			t.Fatalf("checklist step %q out of order or missing: %v", want, ids)
		}
		pos = found
	}
}

func assertNoSecrets(t *testing.T, r coolifyexit.Report) {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"fake-db-password", "fake-session-secret", "fake-webhook-secret"} {
		if strings.Contains(string(b), secret) {
			t.Errorf("report leaks %q", secret)
		}
	}
}

func TestExitReportVerifyModeAndRetire(t *testing.T) {
	_, d := discover(t)
	d.Unsupported, d.Servers = nil, nil
	d.Databases = nil
	d.Apps = d.Apps[:0]
	d.Apps = append(d.Apps, platformimport.App{SourceID: "a1", Name: "solo", Kind: platformimport.SourceImage, Image: "nginx:1", Port: 80})
	plan := platformimport.BuildPlan(d, platformimport.PlanOptions{ExistingApps: map[string]string{"coolify:a1": "solo"}})
	r := coolifyexit.Build(d, plan)
	if r.Mode != coolifyexit.ModeVerify || !r.ReadyToRetire {
		t.Fatalf("mode=%s ready=%v rows=%+v", r.Mode, r.ReadyToRetire, r.Rows)
	}
}
