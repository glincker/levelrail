package incident

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/monitor"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeGuard struct {
	ruleOff bool
	mute    string
}

func (g *fakeGuard) RuleEnabled(context.Context) bool { return !g.ruleOff }
func (g *fakeGuard) Muted(context.Context, string, string, time.Time) (string, bool) {
	return g.mute, g.mute != ""
}

type fakeMonitors struct {
	list   []monitor.Monitor
	status map[string]monitor.Status
}

func (f *fakeMonitors) List(context.Context) ([]monitor.Monitor, error) { return f.list, nil }
func (f *fakeMonitors) Status(id string) monitor.Status                 { return f.status[id] }

func testCfg() Config {
	return Config{Enabled: true, FoldWindow: 10 * time.Minute, ReopenWindow: 15 * time.Minute, SweepInterval: time.Minute, OpenMax: 200, QueueMax: 8}
}

func setup(t *testing.T, cfg Config, g *fakeGuard) (*Opener, *store.DB, *time.Time) {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for id, mon := range map[string]string{"c1": "m1", "c2": "m2"} {
		if err := db.SaveStatusComponent(ctx, store.StatusComponent{ID: id, Kind: "monitor", Target: mon, DisplayName: id}); err != nil {
			t.Fatal(err)
		}
	}
	clock := time.Now().UTC()
	o := New(cfg, db, g, nil)
	o.now = func() time.Time { return clock }
	return o, db, &clock
}

func tr(id, state string) monitor.Transition {
	return monitor.Transition{Monitor: monitor.Monitor{ID: id, Name: "name-" + id}, State: state, Reason: "timeout"}
}

func open(t *testing.T, db *store.DB) []store.InternalStatusIncident {
	t.Helper()
	l, err := db.ListInternalStatusIncidents(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestOpener_OpensUnpublishedAndResolves(t *testing.T) {
	ctx := context.Background()
	o, db, _ := setup(t, testCfg(), &fakeGuard{})
	o.Handle(ctx, tr("m1", monitor.StateDown))
	o.Handle(ctx, tr("m1", monitor.StateDown))
	l := open(t, db)
	if len(l) != 1 || l[0].Published || l[0].InternalState != store.IncidentStateTriggered || l[0].Source != "monitor" {
		t.Fatalf("incidents = %+v", l)
	}
	o.Handle(ctx, tr("m1", monitor.StateUp))
	if got := open(t, db)[0]; got.InternalState != store.IncidentStateResolved || got.Published {
		t.Fatalf("after up = %+v", got)
	}
}

func TestOpener_FoldsSharedGroupAndResolvesWhenAllUp(t *testing.T) {
	ctx := context.Background()
	o, db, _ := setup(t, testCfg(), &fakeGuard{})
	if _, err := db.ExecContext(ctx, `UPDATE status_components SET group_id = 'g1' WHERE id IN ('c1','c2')`); err != nil {
		t.Fatal(err)
	}
	o.Handle(ctx, tr("m1", monitor.StateDown))
	o.Handle(ctx, tr("m2", monitor.StateDown))
	if n := len(open(t, db)); n != 1 {
		t.Fatalf("want 1 folded incident, got %d", n)
	}
	o.Handle(ctx, tr("m1", monitor.StateUp))
	if open(t, db)[0].InternalState != store.IncidentStateTriggered {
		t.Fatal("must stay open while m2 is down")
	}
	o.Handle(ctx, tr("m2", monitor.StateUp))
	if open(t, db)[0].InternalState != store.IncidentStateResolved {
		t.Fatal("must resolve when all links are up")
	}
}

func TestOpener_UngroupedMonitorsDoNotFold(t *testing.T) {
	ctx := context.Background()
	o, db, _ := setup(t, testCfg(), &fakeGuard{})
	o.Handle(ctx, tr("m1", monitor.StateDown))
	o.Handle(ctx, tr("m2", monitor.StateDown))
	if n := len(open(t, db)); n != 2 {
		t.Fatalf("want 2 incidents, got %d", n)
	}
}

func TestOpener_ReopenWithinWindowOnly(t *testing.T) {
	ctx := context.Background()
	o, db, clock := setup(t, testCfg(), &fakeGuard{})
	o.Handle(ctx, tr("m1", monitor.StateDown))
	o.Handle(ctx, tr("m1", monitor.StateUp))
	*clock = clock.Add(5 * time.Minute)
	o.Handle(ctx, tr("m1", monitor.StateDown))
	l := open(t, db)
	if len(l) != 1 || l[0].InternalState != store.IncidentStateTriggered {
		t.Fatalf("want reopen, got %+v", l)
	}
	o.Handle(ctx, tr("m1", monitor.StateUp))
	*clock = clock.Add(time.Hour)
	o.Handle(ctx, tr("m1", monitor.StateDown))
	if n := len(open(t, db)); n != 2 {
		t.Fatalf("after window want a new incident, got %d", n)
	}
}

func TestOpener_MuteAndRuleOffOpenNothing(t *testing.T) {
	ctx := context.Background()
	o, db, _ := setup(t, testCfg(), &fakeGuard{mute: "silence s1"})
	o.Handle(ctx, tr("m1", monitor.StateDown))
	if n := len(open(t, db)); n != 0 {
		t.Fatalf("muted outage opened %d incidents", n)
	}
	o2, db2, _ := setup(t, testCfg(), &fakeGuard{ruleOff: true})
	o2.Handle(ctx, tr("m1", monitor.StateDown))
	if n := len(open(t, db2)); n != 0 {
		t.Fatalf("rule off opened %d incidents", n)
	}
}

func TestOpener_SweepResolvesRecoveredAndDeleted(t *testing.T) {
	ctx := context.Background()
	o, db, _ := setup(t, testCfg(), &fakeGuard{})
	o.Handle(ctx, tr("m1", monitor.StateDown))
	o.Handle(ctx, tr("m9", monitor.StateDown))
	fm := &fakeMonitors{list: []monitor.Monitor{{ID: "m1"}}, status: map[string]monitor.Status{"m1": {State: monitor.StateDown}}}
	o.SetMonitors(fm)
	o.Sweep(ctx)
	states := map[string]string{}
	for _, i := range open(t, db) {
		states[i.SourceRef] = i.InternalState
	}
	if states["m1"] != store.IncidentStateTriggered || states["m9"] != store.IncidentStateResolved {
		t.Fatalf("after first sweep = %v (deleted m9 must resolve, down m1 must stay)", states)
	}
	fm.status["m1"] = monitor.Status{State: monitor.StateUp}
	o.Sweep(ctx)
	for _, i := range open(t, db) {
		if i.InternalState != store.IncidentStateResolved {
			t.Fatalf("incident %s still %s after recovery sweep", i.SourceRef, i.InternalState)
		}
	}
}

func TestOpener_OpenMaxFoldsIntoOverflow(t *testing.T) {
	ctx := context.Background()
	cfg := testCfg()
	cfg.OpenMax = 1
	o, db, _ := setup(t, cfg, &fakeGuard{})
	o.Handle(ctx, tr("m1", monitor.StateDown))
	o.Handle(ctx, tr("mA", monitor.StateDown))
	o.Handle(ctx, tr("mB", monitor.StateDown))
	if n := len(open(t, db)); n != 2 {
		t.Fatalf("want first plus one overflow incident, got %d", n)
	}
}

func TestConfig_DisabledByDefaultAndNeedsBothFlags(t *testing.T) {
	env := map[string]string{"APP_INCIDENTS_ENABLED": "true"}
	look := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	c, err := ConfigFromEnv(look)
	if err != nil || c.Enabled {
		t.Fatalf("incidents alone must not enable: %+v %v", c, err)
	}
	env["APP_MONITORS_ENABLED"] = "true"
	env["APP_INCIDENT_FOLD_WINDOW"] = "2m"
	c, _ = ConfigFromEnv(look)
	if !c.Enabled || c.FoldWindow != 2*time.Minute || c.ReopenWindow != DefaultReopenWindow {
		t.Fatalf("cfg = %+v", c)
	}
	if c, _ = ConfigFromEnv(func(string) (string, bool) { return "", false }); c.Enabled {
		t.Fatal("must default off")
	}
}
