package cleanup_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/cleanup"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

type builtinEnv struct {
	db     *store.DB
	alerts *alerting.DB
	reg    *cleanup.Registry
	now    time.Time
}

func newBuiltinEnv(t *testing.T) *builtinEnv {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	adb, err := alerting.Open(ctx, filepath.Join(t.TempDir(), "alerts.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = adb.Close() })
	e := &builtinEnv{db: db, alerts: adb, reg: cleanup.NewRegistry(nil), now: time.Now().UTC()}
	svc := runtimesettings.New(nil, func(string) (string, bool) { return "", false })
	cleanup.RegisterBuiltins(e.reg, cleanup.BuiltinDeps{Store: db, Alerts: adb, Tokens: db, Janitor: db, Settings: svc, Now: func() time.Time { return e.now }})
	return e
}

func (e *builtinEnv) age(d int) time.Time {
	return e.now.Add(-time.Duration(d)*24*time.Hour - time.Hour)
}

func (e *builtinEnv) seed(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for i, d := range []int{400, 200, 100, 40, 5, 1} {
		a := store.AuditEntry{ID: "aud_" + string(rune('a'+i)), ActorType: "system", Method: "GET", Path: "/x", StatusCode: 200,
			CreatedAt: store.FormatAuditTime(e.age(d)), ClientKind: "system", Action: "sensitive_read"}
		if i%2 == 1 {
			a.Action = ""
		}
		if err := e.db.SaveAuditEntry(ctx, a); err != nil {
			t.Fatal(err)
		}
		if err := e.db.InsertEmailDelivery(ctx, store.EmailDelivery{CreatedAt: e.age(d), Provider: "smtp", Kind: "x", Status: "sent"}, "a@b.c", false); err != nil {
			t.Fatal(err)
		}
		if err := e.db.SaveWebhookDelivery(ctx, store.WebhookDelivery{ID: "whd_" + string(rune('a'+i)), ServiceName: "s", ReceivedAt: e.age(d)}); err != nil {
			t.Fatal(err)
		}
		if err := e.alerts.RecordHistory(ctx, alerting.HistoryEntry{ID: "h" + string(rune('a'+i)), At: e.age(d), RuleID: "r", RuleKind: "threshold", Event: "fired", Outcome: "sent", Severity: "warning"}); err != nil {
			t.Fatal(err)
		}
	}
	now := e.now
	if err := e.db.SaveEventWebhook(ctx, store.EventWebhook{ID: "ew1", Name: "n", URL: "https://example.com", Enabled: true, CreatedAt: store.FormatAuditTime(now), UpdatedAt: store.FormatAuditTime(now)}); err != nil {
		t.Fatal(err)
	}
	for i, d := range []int{100, 40, 5} {
		for _, state := range []string{"delivered", "pending"} {
			ts := store.FormatAuditTime(e.age(d))
			err := e.db.SaveEventDelivery(ctx, store.EventWebhookDelivery{ID: "ed" + state + string(rune('a'+i)), WebhookID: "ew1", EventID: "e", EventType: "t",
				Payload: "{}", State: state, NextAttemptAt: ts, CreatedAt: ts, UpdatedAt: ts})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
}

func TestBuiltinPreviewMatchesRun(t *testing.T) {
	tests := []struct {
		id   string
		days int
		want int64
	}{
		{cleanup.TargetEmailLog, 30, 4},
		{cleanup.TargetWebhookDelivery, 30, 4},
		{cleanup.TargetAlertHistory, 30, 4},
		{cleanup.TargetEventWebhook, 30, 2},
		{cleanup.TargetAuditLog, 90, 2},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			e := newBuiltinEnv(t)
			e.seed(t)
			tg, ok := e.reg.Get(tc.id)
			if !ok {
				t.Fatal("target not registered")
			}
			ctx := context.Background()
			before, err := tg.Measure(ctx)
			if err != nil {
				t.Fatal(err)
			}
			imp, err := tg.Preview(ctx, tc.days)
			if err != nil || imp.Rows != tc.want {
				t.Fatalf("preview = %+v, %v, want %d rows", imp, err, tc.want)
			}
			dry := tg.Run(ctx, tc.days, cleanup.RunOptions{DryRun: true, MaxRows: 1000, Slice: 24 * time.Hour})
			if dry.Err != nil || dry.Rows != tc.want {
				t.Fatalf("dry run = %+v", dry)
			}
			if mid, _ := tg.Measure(ctx); mid.Rows != before.Rows {
				t.Fatalf("a dry run changed the table: %d -> %d rows", before.Rows, mid.Rows)
			}
			res := tg.Run(ctx, tc.days, cleanup.RunOptions{MaxRows: 1000, Slice: 24 * time.Hour})
			if res.Err != nil || res.Rows != imp.Rows || res.More {
				t.Fatalf("run = %+v, preview said %d", res, imp.Rows)
			}
			after, _ := tg.Measure(ctx)
			if after.Rows != before.Rows-res.Rows {
				t.Fatalf("rows %d -> %d after removing %d", before.Rows, after.Rows, res.Rows)
			}
			if again := tg.Run(ctx, tc.days, cleanup.RunOptions{MaxRows: 1000, Slice: 24 * time.Hour}); again.Rows != 0 {
				t.Fatalf("a second run removed %d more rows", again.Rows)
			}
		})
	}
}

func TestAuditKeepsSecurityRowsInsideTheirWindow(t *testing.T) {
	e := newBuiltinEnv(t)
	e.seed(t)
	tg, _ := e.reg.Get(cleanup.TargetAuditLog)
	res := tg.Run(context.Background(), 90, cleanup.RunOptions{MaxRows: 1000, Slice: 24 * time.Hour})
	if res.Err != nil || res.Rows != 2 {
		t.Fatalf("run = %+v", res)
	}
	st, _ := e.db.CleanupStats(context.Background(), store.CleanupAuditLog)
	if st.Rows != 4 {
		t.Fatalf("remaining audit rows = %d, want 4: the 100 day security row stays inside its 365 day window", st.Rows)
	}
}

func TestRowCapStopsARunAndReportsMore(t *testing.T) {
	e := newBuiltinEnv(t)
	e.seed(t)
	tg, _ := e.reg.Get(cleanup.TargetEmailLog)
	res := tg.Run(context.Background(), 1, cleanup.RunOptions{MaxRows: 1, Slice: 24 * time.Hour})
	if res.Err != nil || !res.More || res.Rows < 1 || res.Rows >= 5 {
		t.Fatalf("capped run = %+v", res)
	}
}

func TestTokenTargetPreviewMatchesRun(t *testing.T) {
	e := newBuiltinEnv(t)
	tg, ok := e.reg.Get(cleanup.TargetAPITokens)
	if !ok {
		t.Fatal("token target missing")
	}
	ctx := context.Background()
	imp, err := tg.Preview(ctx, 30)
	if err != nil {
		t.Fatal(err)
	}
	res := tg.Run(ctx, 30, cleanup.RunOptions{MaxRows: 100})
	if res.Err != nil || res.Rows != imp.Rows {
		t.Fatalf("run = %+v, preview = %+v", res, imp)
	}
	if m := tg.Meta(); m.PolicySetting != runtimesettings.KeyRevokedTokenDays || m.Origin != cleanup.OriginExisting {
		t.Fatalf("meta = %+v", m)
	}
}

func TestJanitorStagesAreReadThrough(t *testing.T) {
	e := newBuiltinEnv(t)
	ctx := context.Background()
	want := []string{"docker.containers", "docker.images", "docker.build_cache", "docker.volumes", "deploys.attempts"}
	for _, id := range want {
		tg, ok := e.reg.Get(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		m := tg.Meta()
		if !m.ReadOnly || m.PolicySetting != "" || m.Link == "" {
			t.Errorf("%s meta = %+v", id, m)
		}
		mg, ok := tg.(cleanup.Managed)
		if !ok {
			t.Fatalf("%s reports no state", id)
		}
		st, err := mg.State(ctx)
		if err != nil || st.Mode != cleanup.ModePreview || !st.DryRun {
			t.Errorf("%s state = %+v, %v (the janitor ships in dry run)", id, st, err)
		}
		if res := tg.Run(ctx, 0, cleanup.RunOptions{}); res.Err == nil {
			t.Errorf("%s ran from the cleanup center", id)
		}
	}
}

func TestBuiltinCategoriesAndFloors(t *testing.T) {
	e := newBuiltinEnv(t)
	audit, _ := e.reg.Get(cleanup.TargetAuditLog)
	if m := audit.Meta(); m.Floor != 90 || !m.Destructive || m.PolicySetting != runtimesettings.KeyAuditLogDays {
		t.Fatalf("audit meta = %+v", m)
	}
	for _, tg := range e.reg.Targets() {
		m := tg.Meta()
		if m.Origin != cleanup.OriginExisting {
			t.Errorf("%s: this branch adds only existing sweepers, got origin %s", m.ID, m.Origin)
		}
		if !m.ReadOnly {
			found := false
			for _, d := range runtimesettings.Definitions() {
				found = found || d.Key == m.PolicySetting
			}
			if !found {
				t.Errorf("%s: policy setting %s is not registered", m.ID, m.PolicySetting)
			}
		}
	}
}
