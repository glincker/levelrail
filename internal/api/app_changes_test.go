package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/changes"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestAppChangesEndpoint(t *testing.T) {
	rt, _, cookie := newTimelineRouter(t)
	putApp(t, rt, cookie, `{"image":"nginx:1","port":80,"env":{"A":"1","B":"hunter2"}}`)

	var res changes.Result
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/changes", "", &res); code != http.StatusOK {
		t.Fatalf("changes = %d", code)
	}
	if len(res.Changes) == 0 || res.WindowSec != 1800 {
		t.Fatalf("result = %+v", res)
	}
	var env *changes.Change
	for i := range res.Changes {
		if res.Changes[i].Kind == changes.KindEnv {
			env = &res.Changes[i]
		}
	}
	if env == nil || !strings.Contains(strings.Join(env.Keys, ","), "B") || strings.Contains(env.Title+env.Detail, "hunter2") {
		t.Fatalf("env change = %+v", env)
	}
	if s := res.Suspect(); s == nil {
		t.Fatal("expected a likely cause")
	}

	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/changes?window=junk", "", nil); code != http.StatusBadRequest {
		t.Fatalf("bad window = %d", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/changes?until=yesterday", "", nil); code != http.StatusBadRequest {
		t.Fatalf("bad until = %d", code)
	}
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/ghost/changes", "", nil); code != http.StatusNotFound {
		t.Fatalf("unknown app = %d", code)
	}
	past := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC3339)
	var old changes.Result
	tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/changes?until="+past, "", &old)
	if len(old.Changes) != 0 {
		t.Fatalf("changes before the app existed = %+v", old.Changes)
	}
}

func TestAppChangesRequiresAuth(t *testing.T) {
	rt, _, _ := newTimelineRouter(t)
	for _, target := range []string{"/api/v1/apps/web/changes", "/api/v1/apps/web/slo-preview"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s = %d, want 401", target, rec.Code)
		}
	}
}

func TestDiagnoseCarriesRecentChanges(t *testing.T) {
	db := openTestDB(t)
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))
	rt := NewRouter(logger, testBrand(), db, WithSecretSetter(secrets.NewManager(db, mk)))
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	putApp(t, rt, cookie, `{"image":"nginx:1","port":81,"env":{"A":"1"}}`)

	var d diagnosisResource
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/diagnose", "", &d); code != http.StatusOK {
		t.Fatalf("diagnose = %d", code)
	}
	if d.RecentChanges == nil || len(d.RecentChanges.Changes) == 0 {
		t.Fatalf("diagnose has no recent changes: %+v", d.RecentChanges)
	}
}

func TestAlertHistoryIncludeChanges(t *testing.T) {
	rt, db, adb := newTestRouterWithAlerting(t)
	rt.alertNoise = adb
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")
	if err := db.AddAppEvent(context.Background(), store.AppEvent{AppName: "web", Kind: store.AppEventEnvChange, Title: "Env changed: A", Keys: []string{"A"}, Actor: "bob", CreatedAt: time.Now().Add(-5 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := adb.RecordHistory(context.Background(), alerting.HistoryEntry{RuleID: "r", RuleName: "5xx", App: "web", Event: alerting.EventFired, Outcome: alerting.OutcomeSent, At: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := adb.RecordHistory(context.Background(), alerting.HistoryEntry{RuleID: "r", RuleName: "5xx", App: "web", Event: alerting.EventResolved, Outcome: alerting.OutcomeSent, At: time.Now()}); err != nil {
		t.Fatal(err)
	}

	var plain []alertHistoryResource
	tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/alert-history", "", &plain)
	for _, e := range plain {
		if e.Changes != nil {
			t.Fatal("changes attached without include=changes")
		}
	}
	var with []alertHistoryResource
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/alert-history?include=changes", "", &with); code != http.StatusOK {
		t.Fatalf("history = %d", code)
	}
	fired := 0
	for _, e := range with {
		switch e.Event {
		case alerting.EventFired:
			fired++
			if e.Changes == nil || len(e.Changes.Changes) != 1 || !e.Changes.Changes[0].LikelyCause {
				t.Fatalf("fired entry changes = %+v", e.Changes)
			}
		default:
			if e.Changes != nil {
				t.Fatal("only fired entries carry changes")
			}
		}
	}
	if fired != 1 {
		t.Fatalf("fired entries = %d", fired)
	}
}

func TestSLORuleValidationAndPreview(t *testing.T) {
	rt, db, _ := newTestRouterWithAlerting(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	tests := []struct {
		name string
		body string
		want int
	}{
		{"missing slo", `{"name":"s","kind":"slo_burn","enabled":true}`, http.StatusBadRequest},
		{"target 100", `{"name":"s","kind":"slo_burn","enabled":true,"slo":{"objective":"availability","target":100}}`, http.StatusBadRequest},
		{"latency without threshold", `{"name":"s","kind":"slo_burn","enabled":true,"slo":{"objective":"latency","target":99}}`, http.StatusBadRequest},
		{"valid availability", `{"name":"s","kind":"slo_burn","enabled":true,"severity":"critical","slo":{"objective":"availability","target":99.9}}`, http.StatusCreated},
		{"valid latency", `{"name":"l","kind":"slo_burn","enabled":true,"slo":{"objective":"latency","target":99,"latency_ms":250}}`, http.StatusCreated},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out ruleResource
			if code := tlJSON(t, rt, cookie, http.MethodPost, "/api/v1/apps/web/alerts", tc.body, &out); code != tc.want {
				t.Fatalf("status = %d, want %d", code, tc.want)
			}
			if tc.want == http.StatusCreated && (out.SLO == nil || out.Kind != "slo_burn") {
				t.Fatalf("created rule lost its slo: %+v", out)
			}
		})
	}
	var list []ruleResource
	tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/alerts", "", &list)
	if len(list) != 2 {
		t.Fatalf("rules = %d, want 2", len(list))
	}

	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/slo-preview", "", nil); code != http.StatusNotImplemented {
		t.Fatalf("preview without telemetry = %d, want 501", code)
	}
}

func TestSLOPreviewWithTelemetry(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedApp(t, db, "web")

	var empty sloPreviewResource
	if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/slo-preview", "", &empty); code != http.StatusOK {
		t.Fatalf("preview = %d", code)
	}
	if empty.HasTraffic || empty.BudgetRemaining != 1 || len(empty.Tiers) != 4 {
		t.Fatalf("no-traffic preview = %+v", empty)
	}

	at := time.Now().Add(-2 * time.Minute)
	if err := tdb.WriteSamples(context.Background(), []telemetry.Sample{
		{ResourceID: "service:web", Metric: telemetry.MetricHTTPRequests, Timestamp: at, Value: 1000},
		{ResourceID: "service:web", Metric: telemetry.MetricHTTPResponses5xx, Timestamp: at, Value: 500},
	}); err != nil {
		t.Fatal(err)
	}
	var busy sloPreviewResource
	tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/slo-preview?target=99.5", "", &busy)
	if !busy.HasTraffic || !busy.Page || busy.BudgetRemaining >= 0 || busy.Config.Target != 99.5 {
		t.Fatalf("burning preview = %+v", busy)
	}

	for _, q := range []string{"target=abc", "target=100", "objective=latency", "latency_ms=x", "objective=nope"} {
		if code := tlJSON(t, rt, cookie, http.MethodGet, "/api/v1/apps/web/slo-preview?"+q, "", nil); code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", q, code)
		}
	}
}
