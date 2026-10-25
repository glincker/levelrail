package cleanup_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/cleanup"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const retentionTestDays = 30

type retentionEnv struct {
	db  *store.DB
	reg *cleanup.Registry
	now time.Time
}

func newRetentionEnv(t *testing.T) *retentionEnv {
	t.Helper()
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "main.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	e := &retentionEnv{db: db, reg: cleanup.NewRegistry(nil), now: time.Now().UTC()}
	svc := runtimesettings.New(nil, func(string) (string, bool) { return "", false })
	cleanup.RegisterBuiltins(e.reg, cleanup.BuiltinDeps{Retention: db, Settings: svc, Now: func() time.Time { return e.now }})
	return e
}

func (e *retentionEnv) ago(days int) time.Time {
	return e.now.Add(-time.Duration(days)*24*time.Hour - time.Hour)
}

func (e *retentionEnv) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := e.db.ExecContext(context.Background(), q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func (e *retentionEnv) count(t *testing.T, table string) int {
	t.Helper()
	var n int
	if err := e.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// seedTables inserts rows with foreign keys off because the parents (users, backups, deploys) are not what is under test.
func (e *retentionEnv) seedTables(t *testing.T, fn func(exec func(q string, args ...any))) {
	t.Helper()
	ctx := context.Background()
	conn, err := e.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`) }()
	fn(func(q string, args ...any) {
		if _, err := conn.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	})
}

func (e *retentionEnv) seedAuth(t *testing.T) {
	e.seedTables(t, func(exec func(string, ...any)) {
		for i, d := range []struct {
			expires time.Time
		}{{e.ago(100)}, {e.ago(5)}, {e.now.Add(48 * time.Hour)}} {
			exec(`INSERT INTO theauth_sessions (id, user_id, token_hash, created_at, expires_at) VALUES (?, 'u', ?, ?, ?)`,
				fmt.Sprintf("s%d", i), []byte{byte(i)}, d.expires.Add(-time.Hour).UnixMicro(), d.expires.UnixMicro())
		}
		for i, exp := range []time.Time{e.ago(100), e.ago(5)} {
			exec(`INSERT INTO theauth_device_codes (id, device_code_hash, user_code, status, created_at, expires_at) VALUES (?, ?, ?, 'pending', ?, ?)`,
				fmt.Sprintf("d%d", i), []byte{byte(i)}, fmt.Sprintf("CODE-%d", i), exp.Add(-time.Hour).UnixMicro(), exp.UnixMicro())
			exec(`INSERT INTO theauth_magic_links (id, email, token_hash, expires_at, created_at) VALUES (?, 'a@b.c', ?, ?, ?)`,
				fmt.Sprintf("m%d", i), []byte{byte(i)}, exp.UnixMicro(), exp.Add(-time.Hour).UnixMicro())
		}
	})
}

func (e *retentionEnv) seedAI(t *testing.T) {
	ctx := context.Background()
	mk := func(session string, when time.Time, msgs ...string) {
		if _, err := e.db.CreateAIChatSession(ctx, session, "owner", when); err != nil {
			t.Fatal(err)
		}
		for i, id := range msgs {
			err := e.db.SaveAIChatMessage(ctx, store.AIChatMessage{ID: id, SessionID: session, Role: store.AIChatRoleUser, Content: "x", CreatedAt: when.Add(time.Duration(i) * time.Minute)})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("old", e.ago(100), "old1", "old2")
	err := e.db.SaveAIChatConfirmation(ctx, store.AIChatConfirmation{ID: "c1", SessionID: "old", MessageID: "old1", ToolUseID: "t", ToolName: "n", ToolInput: json.RawMessage(`{}`), Status: "pending", CreatedAt: e.ago(100)})
	if err != nil {
		t.Fatal(err)
	}
	mk("live", e.ago(100), "live1")
	if err := e.db.SaveAIChatMessage(ctx, store.AIChatMessage{ID: "live2", SessionID: "live", Role: store.AIChatRoleUser, Content: "y", CreatedAt: e.ago(2)}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []int{100, 5} {
		if err := e.db.RecordAIUsage(ctx, store.AIUsageRecord{CreatedAt: e.ago(d), Provider: "p", Model: "m"}); err != nil {
			t.Fatal(err)
		}
	}
}

func (e *retentionEnv) seedRestore(t *testing.T) {
	e.seedTables(t, func(exec func(string, ...any)) {
		for i, r := range []struct {
			status string
			when   time.Time
		}{{"succeeded", e.ago(100)}, {"running", e.ago(100)}, {"failed", e.ago(5)}} {
			id := fmt.Sprintf("r%d", i)
			at := r.when.Format(time.RFC3339)
			exec(`INSERT INTO restore_history (id, database_name, backup_history_id, status, started_at) VALUES (?, 'db', 'b', ?, ?)`, id, r.status, at)
			exec(`INSERT INTO pitr_restore_history (id, database_name, base_backup_history_id, target_timestamp, status, started_at) VALUES (?, 'db', 'b', ?, ?, ?)`, id, at, r.status, at)
			status := map[string]string{"succeeded": "passed", "running": "running", "failed": "failed"}[r.status]
			exec(`INSERT INTO backup_drills (id, backup_history_id, resource_kind, status, started_at) VALUES (?, 'b', 'database', ?, ?)`, id, status, at)
		}
	})
}

func (e *retentionEnv) seedProbes(t *testing.T) {
	e.seedTables(t, func(exec func(string, ...any)) {
		for _, d := range []int{100, 5} {
			exec(`INSERT INTO probe_attempts (deploy_attempt_id, target, success, probed_at) VALUES ('a', 't', 1, ?)`, e.ago(d).Format(time.RFC3339Nano))
		}
	})
}

func (e *retentionEnv) seedAppHistory(t *testing.T) {
	ctx := context.Background()
	for i, d := range []int{100, 5} {
		if err := e.db.AddAppEvent(ctx, store.AppEvent{ID: fmt.Sprintf("evt%d", i), AppName: "web", Kind: "restart", CreatedAt: e.ago(d)}); err != nil {
			t.Fatal(err)
		}
		err := e.db.RecordAppScheduleHistory(ctx, store.AppScheduleHistoryEntry{ID: fmt.Sprintf("sch%d", i), ServiceName: "web", ScheduledFor: e.ago(d), FiredAt: e.ago(d), Status: "ok", CreatedAt: e.ago(d)})
		if err != nil {
			t.Fatal(err)
		}
		if err := e.db.SetCanaryOutcome(ctx, store.CanaryOutcome{ServiceName: fmt.Sprintf("svc%d", i), Outcome: "promoted", Reason: "r", Image: "i", At: e.ago(d)}); err != nil {
			t.Fatal(err)
		}
	}
}

func (e *retentionEnv) seedUsage(t *testing.T) {
	for _, d := range []int{100, 5} {
		e.exec(t, `INSERT INTO usage_cost_daily (day, kind, name) VALUES (?, 'app', 'web')`, e.ago(d).Format("2006-01-02"))
	}
}

func (e *retentionEnv) seedIncidents(t *testing.T) {
	ts := func(d int) string { return e.ago(d).Format(time.RFC3339Nano) }
	for i, r := range []struct {
		status string
		days   int
	}{{"resolved", 100}, {"resolved", 5}, {"investigating", 100}, {"scheduled", 100}} {
		e.exec(t, `INSERT INTO status_incidents (id, kind, title, status, starts_at, created_at, updated_at) VALUES (?, 'incident', 't', ?, ?, ?, ?)`,
			fmt.Sprintf("i%d", i), r.status, ts(r.days), ts(r.days), ts(r.days))
	}
	e.exec(t, `INSERT INTO status_incident_updates (id, incident_id, status, body, created_at) VALUES ('u0', 'i0', 'resolved', 'b', ?)`, ts(100))
}

func TestRetentionTargetsPreviewMatchesRun(t *testing.T) {
	tests := []struct {
		id        string
		seed      func(*retentionEnv, *testing.T)
		want      int64
		remaining map[string]int
	}{
		{cleanup.TargetAuthExpired, (*retentionEnv).seedAuth, 3, map[string]int{"theauth_sessions": 2, "theauth_device_codes": 1, "theauth_magic_links": 1}},
		{cleanup.TargetAIHistory, (*retentionEnv).seedAI, 4, map[string]int{"ai_chat_sessions": 1, "ai_chat_messages": 2, "ai_chat_confirmations": 0, "ai_usage": 1}},
		{cleanup.TargetRestoreHistory, (*retentionEnv).seedRestore, 3, map[string]int{"restore_history": 2, "pitr_restore_history": 2, "backup_drills": 2}},
		{cleanup.TargetProbeAttempts, (*retentionEnv).seedProbes, 1, map[string]int{"probe_attempts": 1}},
		{cleanup.TargetAppHistory, (*retentionEnv).seedAppHistory, 3, map[string]int{"app_events": 1, "app_schedule_history": 1, "canary_outcomes": 1}},
		{cleanup.TargetUsageCost, (*retentionEnv).seedUsage, 1, map[string]int{"usage_cost_daily": 1}},
		{cleanup.TargetStatusIncidents, (*retentionEnv).seedIncidents, 1, map[string]int{"status_incidents": 3, "status_incident_updates": 0}},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			e := newRetentionEnv(t)
			tc.seed(e, t)
			tg, ok := e.reg.Get(tc.id)
			if !ok {
				t.Fatal("target not registered")
			}
			ctx := context.Background()
			before, err := tg.Measure(ctx)
			if err != nil || before.Rows == 0 || before.Oldest == nil {
				t.Fatalf("measure = %+v, %v", before, err)
			}
			imp, err := tg.Preview(ctx, retentionTestDays)
			if err != nil || imp.Rows != tc.want {
				t.Fatalf("preview = %+v, %v, want %d rows", imp, err, tc.want)
			}
			dry := tg.Run(ctx, retentionTestDays, cleanup.RunOptions{DryRun: true, MaxRows: 1000, Slice: 24 * time.Hour})
			if dry.Err != nil || dry.Rows != tc.want {
				t.Fatalf("dry run = %+v", dry)
			}
			if mid, _ := tg.Measure(ctx); mid.Rows != before.Rows {
				t.Fatalf("a dry run changed the data: %d -> %d rows", before.Rows, mid.Rows)
			}
			res := tg.Run(ctx, retentionTestDays, cleanup.RunOptions{MaxRows: 100000, Slice: 24 * time.Hour})
			if res.Err != nil || res.Rows != imp.Rows || res.More {
				t.Fatalf("run = %+v, preview said %d", res, imp.Rows)
			}
			for table, n := range tc.remaining {
				if got := e.count(t, table); got != n {
					t.Errorf("%s has %d rows after the run, want %d (young and unfinished rows must survive)", table, got, n)
				}
			}
			if again := tg.Run(ctx, retentionTestDays, cleanup.RunOptions{MaxRows: 100000, Slice: 24 * time.Hour}); again.Rows != 0 {
				t.Fatalf("a second run removed %d more rows", again.Rows)
			}
		})
	}
}

func TestRetentionTargetsAreNewAndPreview(t *testing.T) {
	e := newRetentionEnv(t)
	targets := e.reg.Targets()
	if len(targets) != 7 {
		t.Fatalf("registered %d targets, want 7", len(targets))
	}
	for _, tg := range targets {
		m := tg.Meta()
		if m.Origin != cleanup.OriginNew || cleanup.DefaultMode(m.Origin) != cleanup.ModePreview {
			t.Errorf("%s: origin %s must default to preview", m.ID, m.Origin)
		}
		if m.Floor != 0 || m.Destructive || m.ReadOnly {
			t.Errorf("%s: unexpected floor or flags %+v", m.ID, m)
		}
		var def *runtimesettings.Definition
		for _, d := range runtimesettings.Definitions() {
			if d.Key == m.PolicySetting {
				def = &d
			}
		}
		if def == nil || def.Kind != runtimesettings.KindInt || def.Category != runtimesettings.CategoryRetention {
			t.Errorf("%s: policy setting %s is missing or not a retention int", m.ID, m.PolicySetting)
		}
	}
}

func TestRetentionRowCapStopsARunAndReportsMore(t *testing.T) {
	e := newRetentionEnv(t)
	e.seedTables(t, func(exec func(string, ...any)) {
		for i := range 5 {
			exec(`INSERT INTO probe_attempts (deploy_attempt_id, target, success, probed_at) VALUES ('a', 't', 1, ?)`, e.ago(100+i*3).Format(time.RFC3339Nano))
		}
	})
	tg, _ := e.reg.Get(cleanup.TargetProbeAttempts)
	res := tg.Run(context.Background(), retentionTestDays, cleanup.RunOptions{MaxRows: 1, Slice: 24 * time.Hour})
	if res.Err != nil || !res.More || res.Rows < 1 || res.Rows >= 5 {
		t.Fatalf("capped run = %+v", res)
	}
}

func TestRetentionDeleteRunsInBatches(t *testing.T) {
	e := newRetentionEnv(t)
	e.seedTables(t, func(exec func(string, ...any)) {
		for range 7 {
			exec(`INSERT INTO probe_attempts (deploy_attempt_id, target, success, probed_at) VALUES ('a', 't', 1, ?)`, e.ago(100).Format(time.RFC3339Nano))
		}
	})
	n, err := e.db.RetentionDeleteOlder(context.Background(), store.RetentionProbeAttempts, e.ago(retentionTestDays), 3)
	if err != nil || n != 7 {
		t.Fatalf("deleted %d, %v, want 7 across batches of 3", n, err)
	}
}
