package platformimport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestSanitizeName(t *testing.T) {
	long := strings.Repeat("a", 60)
	cases := map[string]string{
		"My Web App":  "my-web-app",
		"9lives":      "app-9lives",
		"--x--":       "x",
		"":            "imported",
		"Under_Score": "under-score",
		long:          strings.Repeat("a", 40),
	}
	for in, want := range cases {
		if got := SanitizeName(in); got != want {
			t.Errorf("SanitizeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLooksSecret(t *testing.T) {
	cases := map[string]bool{
		"DB_PASSWORD": true, "API_KEY": true, "SESSION_SECRET": true, "DATABASE_URL": true, "AUTH_TOKEN": true,
		"NODE_ENV": false, "PORT": false, "LOG_LEVEL": false, "MONKEY": false,
	}
	for k, want := range cases {
		if got := LooksSecret(k); got != want {
			t.Errorf("LooksSecret(%q) = %v, want %v", k, got, want)
		}
	}
}

func TestParseMemory(t *testing.T) {
	cases := map[string]int64{"512m": 512 << 20, "1g": 1 << 30, "0": 0, "": 0, "abc": 0, "2048": 2048, "1.5G": 3 << 29}
	for in, want := range cases {
		if got := parseMemory(in); got != want {
			t.Errorf("parseMemory(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestNetworkPolicy(t *testing.T) {
	cases := []struct {
		name   string
		policy NetworkPolicy
		addr   string
		blocks bool
	}{
		{"public", NetworkPolicy{}, "8.8.8.8", false},
		{"private default", NetworkPolicy{}, "10.0.0.5", true},
		{"private allowed", NetworkPolicy{AllowPrivate: true}, "192.168.1.10", false},
		{"cgnat default", NetworkPolicy{}, "100.64.0.1", true},
		{"loopback default", NetworkPolicy{}, "127.0.0.1", true},
		{"loopback private only", NetworkPolicy{AllowPrivate: true}, "127.0.0.1", true},
		{"loopback allowed", NetworkPolicy{AllowLoopback: true}, "127.0.0.1", false},
		{"metadata never", NetworkPolicy{AllowPrivate: true, AllowLoopback: true}, "169.254.169.254", true},
		{"metadata v6 mapped", NetworkPolicy{AllowPrivate: true, AllowLoopback: true}, "::ffff:169.254.169.254", true},
		{"aws v6 metadata", NetworkPolicy{AllowPrivate: true}, "fd00:ec2::254", true},
		{"alibaba metadata", NetworkPolicy{AllowPrivate: true}, "100.100.100.200", true},
		{"unspecified", NetworkPolicy{AllowPrivate: true, AllowLoopback: true}, "0.0.0.0", true},
	}
	for _, c := range cases {
		err := c.policy.Check(netip.MustParseAddr(c.addr))
		if (err != nil) != c.blocks {
			t.Errorf("%s: err=%v, blocks=%v", c.name, err, c.blocks)
		}
	}
}

func TestClientBlocksLoopbackByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("[]")) }))
	defer srv.Close()
	src, err := NewCoolify(srv.URL, "tok", ClientOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = src.Discover(context.Background())
	if !errors.Is(err, ErrBlockedAddress) && (err == nil || !strings.Contains(err.Error(), "not allowed")) {
		t.Fatalf("want blocked address error, got %v", err)
	}
}

func TestRequesterRejectsBadURLs(t *testing.T) {
	for _, u := range []string{"", "ftp://x", "https://user:pw@host", "not a url", "https://"} {
		if _, err := NewCoolify(u, "t", testOpts); err == nil {
			t.Errorf("want error for %q", u)
		}
	}
}

func TestResponseSizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBytes+10)))
	}))
	defer srv.Close()
	src, _ := NewCoolify(srv.URL, "tok", testOpts)
	if _, err := src.Discover(context.Background()); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("want size error, got %v", err)
	}
}

func sampleDiscovery() *Discovery {
	return &Discovery{Platform: Dokploy,
		Apps: []App{
			{SourceID: "a1", Name: "Web", Kind: SourceImage, Image: "nginx:1", Port: 8080, Replicas: 2, Project: "P",
				Env:     []Env{{Key: "MODE", Value: "x"}, {Key: "API_KEY", Value: "k", Secret: true}, {Key: "EMPTY_SECRET", Secret: true}},
				Volumes: []Volume{{Name: "data", ContainerPath: "/d"}, {HostPath: "/srv/x", ContainerPath: "/x"}},
				Health:  &HealthCheck{Path: "/h", IntervalSeconds: 5}, Crons: []Cron{{Name: "n", Schedule: "* * * * *", Command: "run"}}},
			{SourceID: "a2", Name: "git app", Kind: SourceGit, GitURL: "https://github.com/a/b", GitBranch: "main", BuildMethod: "dockerfile"},
			{SourceID: "a3", Name: "plain", Kind: SourceUnknown},
		},
		Databases:   []Database{{SourceID: "d1", Name: "Main DB", Engine: "postgres", Version: "16"}, {SourceID: "d2", Name: "mystery"}},
		Unsupported: []Unsupported{{Kind: "compose", SourceID: "c1", Name: "stack", Reason: "multi", Manual: "do it"}},
	}
}

func findItem(t *testing.T, r Report, kind, id string) Item {
	t.Helper()
	for _, it := range r.Items {
		if it.Kind == kind && it.SourceID == id {
			return it
		}
	}
	t.Fatalf("no item %s %s", kind, id)
	return Item{}
}

func TestBuildPlanMapping(t *testing.T) {
	p := BuildPlan(sampleDiscovery(), PlanOptions{})
	if len(p.Apps) != 3 || len(p.Databases) != 1 {
		t.Fatalf("apps=%d dbs=%d", len(p.Apps), len(p.Databases))
	}
	web := p.Apps[0]
	if web.Name != "web" || web.Image != "nginx:1" || web.Port != 8080 || web.Replicas != 2 || web.Project != "P" {
		t.Errorf("web: %+v", web)
	}
	if web.Env["MODE"] != "x" || web.Secrets["API_KEY"] != "k" || len(web.Secrets) != 1 || web.Env["API_KEY"] != "" {
		t.Errorf("env split: env=%v secrets=%d", web.Env, len(web.Secrets))
	}
	if web.Labels[LabelSourceID] != "dokploy:a1" || web.Labels[LabelSourceName] != "Web" {
		t.Errorf("labels: %v", web.Labels)
	}
	if len(web.Volumes) != 2 || web.Volumes[0].Name != "data" || web.Health == nil || web.Health.Path != "/h" {
		t.Errorf("volumes/health: %+v", web)
	}
	it := findItem(t, p.Report, "app", "a1")
	if it.Status != StatusNeedsAttention || len(it.Reasons) < 3 {
		t.Errorf("a1 item: %+v", it)
	}
	if g := p.Apps[1]; g.Image != "git-app:pending" || g.GitURL != "https://github.com/a/b" {
		t.Errorf("git app: %+v", g)
	}
	if it := findItem(t, p.Report, "app", "a3"); it.Status != StatusNeedsAttention {
		t.Errorf("port defaulting must flag attention: %+v", it)
	}
	if it := findItem(t, p.Report, "database", "d2"); it.Status != StatusUnsupported {
		t.Errorf("mystery db: %+v", it)
	}
	if it := findItem(t, p.Report, "compose", "c1"); it.Status != StatusUnsupported || len(it.Manual) != 1 {
		t.Errorf("compose: %+v", it)
	}
	if len(p.Report.Notes) != 1 || !strings.Contains(p.Report.Notes[0], "empty") {
		t.Errorf("data note: %v", p.Report.Notes)
	}
	if p.Report.Counts[StatusUnsupported] != 2 {
		t.Errorf("counts: %v", p.Report.Counts)
	}
}

func TestBuildPlanCollisionAndIdempotency(t *testing.T) {
	d := sampleDiscovery()
	d.Apps = d.Apps[:1]
	d.Databases = d.Databases[:1]

	suffix := BuildPlan(d, PlanOptions{TakenApps: map[string]bool{"web": true}, ExistingDatabases: map[string]string{"main-db": "mysql:8"}})
	if suffix.Apps[0].Name != "web-2" || suffix.Databases[0].Name != "main-db-2" {
		t.Errorf("suffix: %s %s", suffix.Apps[0].Name, suffix.Databases[0].Name)
	}

	skip := BuildPlan(d, PlanOptions{Collision: CollisionSkip, TakenApps: map[string]bool{"web": true}, ExistingDatabases: map[string]string{"main-db": "mysql:8"}})
	if len(skip.Apps) != 0 || len(skip.Databases) != 0 || skip.Report.Counts[StatusSkipped] != 2 {
		t.Errorf("skip: %+v", skip.Report)
	}

	again := BuildPlan(d, PlanOptions{
		ExistingApps:      map[string]string{"dokploy:a1": "web"},
		TakenApps:         map[string]bool{"web": true},
		ExistingDatabases: map[string]string{"main-db": "postgres:16"},
	})
	if len(again.Apps) != 0 || len(again.Databases) != 0 || again.Report.Counts[StatusAlready] != 2 {
		t.Errorf("rerun: %+v", again.Report)
	}
}

func TestBuildPlanOnlyAndDedupe(t *testing.T) {
	d := sampleDiscovery()
	d.Apps = append(d.Apps, d.Apps[0])
	p := BuildPlan(d, PlanOptions{Only: []string{"a1", "git app", "d1"}})
	if len(p.Apps) != 2 || len(p.Databases) != 1 || len(p.Report.Items) != 3 {
		t.Errorf("only: apps=%d dbs=%d items=%d", len(p.Apps), len(p.Databases), len(p.Report.Items))
	}
	same := BuildPlan(&Discovery{Platform: Coolify, Apps: []App{{SourceID: "x", Name: "same", Kind: SourceImage, Image: "i", Port: 1}, {SourceID: "y", Name: "same", Kind: SourceImage, Image: "i", Port: 1}}}, PlanOptions{})
	if same.Apps[0].Name != "same" || same.Apps[1].Name != "same-2" {
		t.Errorf("in-batch collision: %s %s", same.Apps[0].Name, same.Apps[1].Name)
	}
}

type fakeApplier struct {
	failApp map[string]bool
	created []string
}

func (f *fakeApplier) CreateApp(_ context.Context, p AppPlan) ([]string, error) {
	if f.failApp[p.Name] {
		return nil, errors.New("boom")
	}
	f.created = append(f.created, p.Name)
	return nil, nil
}

func (f *fakeApplier) CreateDatabase(_ context.Context, p DatabasePlan) ([]string, error) {
	f.created = append(f.created, p.Name)
	return nil, nil
}

func TestApplyPartialFailureThenResume(t *testing.T) {
	d := sampleDiscovery()
	fa := &fakeApplier{failApp: map[string]bool{"git-app": true}}
	plan := BuildPlan(d, PlanOptions{})
	rep := Apply(context.Background(), plan, fa)
	if rep.Counts[StatusFailed] != 1 || rep.Counts[StatusCreated] != 3 {
		t.Fatalf("counts: %v", rep.Counts)
	}
	if it := findItem(t, rep, "app", "a2"); it.Status != StatusFailed || !strings.Contains(strings.Join(it.Reasons, " "), "boom") {
		t.Errorf("failed item: %+v", it)
	}
	if plan.Report.Counts[StatusFailed] != 0 {
		t.Error("Apply must not mutate the plan's report")
	}

	existing := map[string]string{"dokploy:a1": "web", "dokploy:a3": "plain"}
	taken := map[string]bool{"web": true, "plain": true}
	fa2 := &fakeApplier{}
	plan2 := BuildPlan(d, PlanOptions{ExistingApps: existing, TakenApps: taken, ExistingDatabases: map[string]string{"main-db": "postgres:16"}})
	rep2 := Apply(context.Background(), plan2, fa2)
	if len(fa2.created) != 1 || fa2.created[0] != "git-app" || rep2.Counts[StatusAlready] != 3 {
		t.Errorf("resume created=%v counts=%v", fa2.created, rep2.Counts)
	}
}

func TestApplyStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rep := Apply(ctx, BuildPlan(sampleDiscovery(), PlanOptions{}), &fakeApplier{})
	if rep.Counts[StatusFailed] != 4 {
		t.Errorf("counts: %v", rep.Counts)
	}
}

func TestPlanNeverSerializesSecrets(t *testing.T) {
	p := BuildPlan(sampleDiscovery(), PlanOptions{})
	if p.Apps[0].Secrets["API_KEY"] != "k" {
		t.Fatal("secret should be held for apply")
	}
}
