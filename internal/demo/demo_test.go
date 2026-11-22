package demo

import (
	"context"
	"errors"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/store/storetest"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

var testAnchor = time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)

func seeded(t *testing.T, seed int64) *store.DB {
	t.Helper()
	db := storetest.Open(t)
	if _, err := Seed(context.Background(), db, Options{Seed: seed, Anchor: testAnchor}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	return db
}

func TestSeedCoreEntities(t *testing.T) {
	ctx := context.Background()
	db := seeded(t, 1)

	svcs, err := db.ListDesiredServices(ctx)
	if err != nil || len(svcs) != len(appSpecs)+len(pagesSites) {
		t.Fatalf("services = %d, err = %v, want %d", len(svcs), err, len(appSpecs)+len(pagesSites))
	}
	dbs, err := db.ListDesiredDatabases(ctx)
	if err != nil || len(dbs) != len(databaseSpecs) {
		t.Fatalf("databases = %d, err = %v, want %d", len(dbs), err, len(databaseSpecs))
	}
	if org, err := db.GetOrganization(ctx, store.DefaultTeamID); err != nil || org.Name != "Acme Cloud" {
		t.Fatalf("default org = %+v, err = %v, want the renamed seeded org", org, err)
	}
	nodes, err := db.ListNodes(ctx)
	if err != nil || len(nodes) != len(nodeSpecs) {
		t.Fatalf("nodes = %d, err = %v, want %d", len(nodes), err, len(nodeSpecs))
	}

	attempts, err := db.ListDeployAttempts(ctx, incidentApp)
	if err != nil {
		t.Fatalf("ListDeployAttempts() error = %v", err)
	}
	var rollback bool
	for _, a := range attempts {
		if a.Source == store.DeployAttemptSourceAutoRollback {
			rollback = true
		}
	}
	if !rollback {
		t.Errorf("%s has no auto rollback attempt in its history", incidentApp)
	}

	users, err := db.ListUsers(ctx)
	if err != nil {
		t.Fatalf("ListUsers() error = %v", err)
	}
	for _, u := range users {
		if !IsSeededUserID(u.ID) {
			t.Errorf("user %q is not recognised as seeded", u.ID)
		}
		if u.ID == VisitorUserID && (len(u.Abilities) != 1 || u.Abilities[0] != "read") {
			t.Errorf("visitor abilities = %v, want [read]", u.Abilities)
		}
	}
}

func TestSeedDeterministic(t *testing.T) {
	ctx := context.Background()
	a, b := seeded(t, 7), seeded(t, 7)
	type row struct{ Name, SHA, Status, Source string }
	collect := func(db *store.DB) []row {
		var out []row
		for _, spec := range appSpecs {
			list, err := db.ListDeployAttempts(ctx, spec.name)
			if err != nil {
				t.Fatalf("ListDeployAttempts() error = %v", err)
			}
			for _, d := range list {
				out = append(out, row{d.ID, d.CommitSHA, d.Status, d.Source})
			}
		}
		return out
	}
	if !reflect.DeepEqual(collect(a), collect(b)) {
		t.Error("two runs with the same seed produced different deploy history")
	}
	c := seeded(t, 8)
	if reflect.DeepEqual(collect(a), collect(c)) {
		t.Error("different seeds produced identical deploy history")
	}
}

func TestSeedIdempotentAndGuarded(t *testing.T) {
	ctx := context.Background()
	db := seeded(t, 3)

	if _, err := Seed(ctx, db, Options{Seed: 3, Anchor: testAnchor}); err != nil {
		t.Errorf("re-seed with identical options error = %v, want nil", err)
	}
	if _, err := Seed(ctx, db, Options{Seed: 4, Anchor: testAnchor}); !errors.Is(err, ErrSeedMismatch) {
		t.Errorf("re-seed with other seed error = %v, want ErrSeedMismatch", err)
	}

	live := storetest.Open(t)
	if err := live.CreateUser(ctx, store.User{ID: "user_real", Email: "owner", DisplayName: "owner", Abilities: []string{"root"}, IsFirstUser: true, CreatedAt: testAnchor}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if _, err := Seed(ctx, live, Options{Seed: 1, Anchor: testAnchor}); !errors.Is(err, ErrNotEmpty) {
		t.Errorf("seed over real users error = %v, want ErrNotEmpty", err)
	}
	if err := CheckStore(ctx, live); !errors.Is(err, ErrNotDemoInstance) {
		t.Errorf("CheckStore(live) error = %v, want ErrNotDemoInstance", err)
	}
}

func TestCheckStoreRejectsForeignUser(t *testing.T) {
	ctx := context.Background()
	db := seeded(t, 1)
	if err := CheckStore(ctx, db); err != nil {
		t.Fatalf("CheckStore(seeded) error = %v", err)
	}
	if err := db.CreateUser(ctx, store.User{ID: "user_intruder", Email: "x", DisplayName: "x", Abilities: []string{"root"}, CreatedAt: testAnchor}); err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}
	if err := CheckStore(ctx, db); err == nil {
		t.Error("CheckStore with a non-seeded user = nil, want error")
	}
}

func TestCheckEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{"no opt in", map[string]string{}, true},
		{"opt in", map[string]string{EnvDemoMode: "1"}, false},
		{"opt in false value", map[string]string{EnvDemoMode: "0"}, true},
		{"docker host set", map[string]string{EnvDemoMode: "1", "DOCKER_HOST": "unix:///var/run/docker.sock"}, true},
		{"agent listener set", map[string]string{EnvDemoMode: "1", "APP_AGENT_ADDR": ":9443"}, true},
		{"ingress listener set", map[string]string{EnvDemoMode: "1", "APP_INGRESS_HTTPS_ADDR": ":443"}, true},
		{"loopback bind", map[string]string{EnvDemoMode: "1", "APP_HTTP_ADDR": "127.0.0.1:8080"}, false},
		{"public bind", map[string]string{EnvDemoMode: "1", "APP_HTTP_ADDR": "0.0.0.0:8080"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckEnvironmentIn(func(k string) string { return tt.env[k] }, InContainer, noSocket)
			if (err != nil) != tt.wantErr {
				t.Errorf("CheckEnvironment() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

type fakePub struct{ n int }

func (f *fakePub) Publish(telemetry.LogEntry) { f.n++ }

type fakeSamples struct{ got []telemetry.Sample }

func (f *fakeSamples) WriteSamples(_ context.Context, s []telemetry.Sample) error {
	f.got = append(f.got, s...)
	return nil
}

func TestGeneratorTicks(t *testing.T) {
	pub, ws := &fakePub{}, &fakeSamples{}
	g := &Generator{Services: []string{"a", "b", "c"}, Logs: pub, Samples: ws, Seed: 1}
	g.TickLogs(testAnchor)
	if pub.n != 3 {
		t.Errorf("published = %d, want 3", pub.n)
	}
	if err := g.TickSamples(context.Background(), testAnchor); err != nil {
		t.Fatalf("TickSamples() error = %v", err)
	}
	if len(ws.got) != 6 {
		t.Errorf("samples = %d, want 6", len(ws.got))
	}
}

func TestDockerFakes(t *testing.T) {
	ctx := context.Background()
	f := NewDocker(seeded(t, 1), func() time.Time { return testAnchor })
	if err := f.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
	cs, err := f.ListByPrefix(ctx, "")
	if err != nil || len(cs) != len(appSpecs) {
		t.Errorf("ListByPrefix() = %d, err = %v, want %d", len(cs), err, len(appSpecs))
	}
	for _, c := range cs {
		if c.Name == "legacy-cms" && c.Running {
			t.Error("suspended service reported running")
		}
	}
	if _, err := f.DiskUsage(ctx); err != nil {
		t.Errorf("DiskUsage() error = %v", err)
	}
}

func TestSeedReadiness(t *testing.T) {
	ctx := context.Background()
	db := seeded(t, 1)
	names := make([]string, len(appSpecs))
	for i, a := range appSpecs {
		names[i] = "application/" + a.name
	}
	conds, err := db.GetConditionsForControllers(ctx, names)
	if err != nil {
		t.Fatalf("GetConditionsForControllers() error = %v", err)
	}
	ready := 0
	for _, a := range appSpecs {
		cs := conds["application/"+a.name]
		if len(cs) != 1 || cs[0].Type != "Ready" {
			t.Fatalf("%s conditions = %+v, want one Ready", a.name, cs)
		}
		if cs[0].Status == "True" {
			ready++
			if !cs[0].LastTransitionTime.Before(testAnchor) {
				t.Errorf("%s ready since %v, want before anchor", a.name, cs[0].LastTransitionTime)
			}
			attempts, err := db.ListDeployAttempts(ctx, a.name)
			if err != nil || len(attempts) == 0 {
				t.Fatalf("attempts for %s: %d, %v", a.name, len(attempts), err)
			}
			serving := false
			for _, at := range attempts {
				serving = serving || at.RolloutState == store.RolloutStateServing
			}
			if !serving {
				t.Errorf("%s has no serving attempt", a.name)
			}
		}
	}
	if want := len(appSpecs) - 2; ready != want {
		t.Errorf("ready apps = %d, want %d", ready, want)
	}
	if c := conds["application/"+crashloopApp][0]; c.Status != "False" || c.Reason != "CrashLoopBackOff" {
		t.Errorf("%s condition = %+v, want failing", crashloopApp, c)
	}
	if c := conds["application/legacy-cms"][0]; c.Reason != "Suspended" {
		t.Errorf("legacy-cms condition = %+v, want Suspended", c)
	}
}

func TestSeedCalmScenario(t *testing.T) {
	ctx := context.Background()
	db := storetest.Open(t)
	if _, err := Seed(ctx, db, Options{Seed: 1, Anchor: testAnchor, Scenario: ScenarioCalm}); err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
	for _, a := range appSpecs {
		conds, err := db.GetConditionsForControllers(ctx, []string{"application/" + a.name})
		if err != nil {
			t.Fatalf("conditions for %s: %v", a.name, err)
		}
		c := conds["application/"+a.name][0]
		if c.Status != "True" && c.Reason != "Suspended" {
			t.Errorf("%s condition = %+v, want ready or stopped", a.name, c)
		}
		attempts, err := db.ListDeployAttempts(ctx, a.name)
		if err != nil {
			t.Fatalf("attempts for %s: %v", a.name, err)
		}
		for _, at := range attempts {
			if at.Status == store.DeployAttemptStatusFailed {
				t.Errorf("%s has failed attempt %s in calm scenario", a.name, at.ID)
			}
		}
	}
	list, err := db.ListDeployApprovals(ctx, store.DeployApprovalStatusPending, "")
	if err != nil {
		t.Fatalf("ListDeployApprovals() error = %v", err)
	}
	pending := len(list)
	if pending > 1 {
		t.Errorf("pending approvals = %d, want at most 1", pending)
	}
}

func TestSeedUnknownScenario(t *testing.T) {
	db := storetest.Open(t)
	if _, err := Seed(context.Background(), db, Options{Seed: 1, Anchor: testAnchor, Scenario: "chaos"}); err == nil {
		t.Fatal("Seed() with unknown scenario succeeded, want error")
	}
}

func TestDemoDigestIsHexSHA256(t *testing.T) {
	d := demoDigest("acme/web", "a41c9e2")
	if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(d) {
		t.Fatalf("digest %q is not sha256 hex", d)
	}
	if strings.Trim(strings.TrimPrefix(d, "sha256:"), "0123456789") == "" {
		t.Fatalf("digest %q is all digits", d)
	}
	if d != demoDigest("acme/web", "a41c9e2") {
		t.Fatal("digest not deterministic")
	}
}

func TestSeedFleet(t *testing.T) {
	ctx := context.Background()
	db := seeded(t, 1)
	nodes, err := db.ListNodes(ctx)
	if err != nil {
		t.Fatalf("ListNodes() error = %v", err)
	}
	var aws, arm64Sites int
	for _, n := range nodes {
		if n.Provider == "aws" && n.Region != "" {
			aws++
		}
		labels, _ := db.GetNodeLabels(ctx, n.ID)
		if n.AgentArch == "arm64" && labels["site"] != "" {
			arm64Sites++
		}
	}
	if aws != 1 || arm64Sites != 3 {
		t.Errorf("aws nodes = %d, site devices = %d, want 1 and 3", aws, arm64Sites)
	}
	batches := map[string]bool{}
	for _, a := range appSpecs {
		if !strings.HasPrefix(a.name, kioskPrefix) {
			continue
		}
		list, err := db.ListDeployAttempts(ctx, a.name)
		if err != nil || len(list) == 0 {
			t.Fatalf("attempts for %s: %d, %v", a.name, len(list), err)
		}
		last := list[0]
		if last.Status != store.DeployAttemptStatusSucceeded || !strings.HasPrefix(last.Reason, "apps rollout") {
			t.Errorf("%s last attempt = %s %q, want a succeeded rollout", a.name, last.Status, last.Reason)
		}
		batches[last.Reason] = true
	}
	if len(batches) != 2 {
		t.Errorf("rollout batches = %v, want 2", batches)
	}
	tpls, err := db.ListCustomTemplates(ctx)
	if err != nil || len(tpls) != len(customTemplates) {
		t.Errorf("templates = %d, err = %v", len(tpls), err)
	}
}

func TestSeedScreensHaveBackingData(t *testing.T) {
	ctx := context.Background()
	db := seeded(t, 1)
	for _, site := range pagesSites {
		svc, err := db.GetDesiredService(ctx, site.name)
		if err != nil || svc.ProjectID == "" {
			t.Errorf("static site %q has no backing service with a project: %v", site.name, err)
		}
	}
	for _, a := range appSpecs {
		svc, err := db.GetDesiredService(ctx, a.name)
		if err != nil || svc.ProjectID != a.project || svc.EnvironmentID == "" {
			t.Errorf("app %q project = %q env = %q err = %v", a.name, svc.ProjectID, svc.EnvironmentID, err)
		}
	}
	g, err := db.GetGitSource(ctx, "storefront-web")
	if err != nil || !g.PreviewEnabled {
		t.Errorf("storefront-web git source previews on = %v, err = %v", g.PreviewEnabled, err)
	}
	if drills, err := db.ListBackupDrills(ctx, "", "", "pg-main", 10); err != nil || len(drills) == 0 {
		t.Errorf("drills = %d, err = %v", len(drills), err)
	}
}
