package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func newScopedListFixture(t *testing.T) (*Router, string, *http.Cookie) {
	t.Helper()
	db := openTestDB(t)
	adb := newTestAlertingDB(t)
	tdb := newTestTelemetryDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db,
		WithPipelines(db, &fakePipelineRunner{}),
		WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)),
		WithAlertRules(adb), WithNotificationChannels(adb), WithAlertNoise(adb),
	)
	ctx := context.Background()
	now := time.Now().UTC()

	for _, app := range []string{"alpha-app", "beta-app"} {
		short := strings.TrimSuffix(app, "-app")
		if err := db.SaveDesiredService(ctx, store.DesiredService{Name: app, Image: "img:1", Port: 80, Domains: []string{short + ".example.com"}}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: short + "-db", Engine: "postgres", Version: "16"}); err != nil {
			t.Fatal(err)
		}
		if err := db.SaveStaticSite(ctx, store.StaticSite{Name: app, Domains: []string{short + "-static.example.com"}, RootDir: "/tmp/" + short}); err != nil {
			t.Fatal(err)
		}
		seedCert(t, db, short+".example.com", now.Add(-time.Hour), now.Add(90*24*time.Hour))
		if err := adb.RecordHistory(ctx, alerting.HistoryEntry{RuleID: "r-" + short, RuleName: "rule-" + short, RuleKind: "app", App: app, Event: "fired", Outcome: "sent"}); err != nil {
			t.Fatal(err)
		}
	}
	seedCert(t, db, "platform.example.com", now.Add(-time.Hour), now.Add(90*24*time.Hour))
	if err := adb.RecordHistory(ctx, alerting.HistoryEntry{RuleID: "r-plat", RuleName: "rule-platform", RuleKind: "node", Event: "fired", Outcome: "sent"}); err != nil {
		t.Fatal(err)
	}

	target := seedBackupTargetForAPI(t, db)
	for i, b := range []store.BackupHistory{
		{ID: "bk-beta-db", DatabaseName: "beta-db"},
		{ID: "bk-alpha-db", DatabaseName: "alpha-db"},
		{ID: "bk-beta-vol", ResourceKind: store.BackupResourceKindVolume, ServiceName: "beta-app", VolumeName: "data"},
		{ID: "bk-alpha-vol", ResourceKind: store.BackupResourceKindVolume, ServiceName: "alpha-app", VolumeName: "data"},
	} {
		b.TargetID = target.ID
		b.StartedAt = now.Add(-time.Duration(i+1) * time.Minute).Format(time.RFC3339)
		if err := db.StartBackupHistory(ctx, b); err != nil {
			t.Fatal(err)
		}
	}

	seedOverviewRuns(t, db, []overviewSeed{
		{"run-alpha", "alpha-app", "ci-alpha", "push", store.PipelineStatusRunning, "", time.Minute},
		{"run-beta", "beta-app", "ci-beta", "push", store.PipelineStatusRunning, "", 2 * time.Minute},
	})

	tok := seedMatrixToken(t, db, "scoped-tok", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-beta-app", "Deny", "*", "app:beta-app", store.PrincipalTypeToken, "scoped-tok")
	attachTestPolicy(t, db, "deny-beta-db", "Deny", "*", "database:beta-db", store.PrincipalTypeToken, "scoped-tok")
	return rt, tok, loginTestSession(t, rt, db)
}

func TestCrossAppListsScopedToReadableApps(t *testing.T) {
	rt, tok, cookie := newScopedListFixture(t)

	tests := []struct {
		name    string
		path    string
		visible []string
		hidden  []string
	}{
		{"pipeline runs", "/api/v1/pipeline-runs", []string{"run-alpha"}, []string{"run-beta", "beta-app"}},
		{"certificates", "/api/v1/certificates", []string{"alpha.example.com", "platform.example.com"}, []string{"beta.example.com"}},
		{"domains", "/api/v1/domains", []string{"alpha.example.com"}, []string{"beta.example.com", "beta-app"}},
		{"backups", "/api/v1/backups", []string{"bk-alpha-db", "bk-alpha-vol"}, []string{"bk-beta-db", "bk-beta-vol", "beta-db", "beta-app"}},
		{"backups paged past hidden", "/api/v1/backups?limit=1", []string{"bk-alpha-db"}, []string{"bk-beta-db", "bk-beta-vol"}},
		{"static sites", "/api/v1/static-sites", []string{"alpha-static.example.com"}, []string{"beta-static.example.com", "beta-app"}},
		{"alert history", "/api/v1/alert-history", []string{"rule-alpha", "rule-platform"}, []string{"rule-beta", "beta-app"}},
		{"app resource usage", "/api/v1/apps/resource-usage", []string{"alpha-app"}, []string{"beta-app"}},
		{"databases", "/api/v1/databases", []string{"alpha-db"}, []string{"beta-db"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scoped := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			bearer(tok)(req)
			rt.Handler().ServeHTTP(scoped, req)
			if scoped.Code != http.StatusOK {
				t.Fatalf("scoped status = %d, body = %s", scoped.Code, scoped.Body.String())
			}
			body := scoped.Body.String()
			for _, want := range tc.visible {
				if !strings.Contains(body, want) {
					t.Errorf("scoped body missing %q: %s", want, body)
				}
			}
			for _, leak := range tc.hidden {
				if strings.Contains(body, leak) {
					t.Errorf("scoped body leaks %q: %s", leak, body)
				}
			}

			full := httptest.NewRecorder()
			rt.Handler().ServeHTTP(full, authedRequest(t, cookie, http.MethodGet, tc.path, ""))
			if full.Code != http.StatusOK {
				t.Fatalf("admin status = %d, body = %s", full.Code, full.Body.String())
			}
			if !strings.Contains(full.Body.String(), tc.hidden[0]) {
				t.Errorf("admin body missing %q, fixture is not seeding both sides: %s", tc.hidden[0], full.Body.String())
			}
		})
	}
}

func TestPipelineSummaryCountsOnlyReadableApps(t *testing.T) {
	rt, tok, cookie := newScopedListFixture(t)

	var scoped, full pipelineSummaryResource
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/pipelines/summary", nil)
	bearer(tok)(req)
	rt.Handler().ServeHTTP(rec, req)
	if err := json.Unmarshal(rec.Body.Bytes(), &scoped); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/pipelines/summary", ""))
	if err := json.Unmarshal(rec.Body.Bytes(), &full); err != nil {
		t.Fatal(err)
	}
	if scoped.Running != 1 || full.Running != 2 {
		t.Fatalf("running scoped = %d (want 1), admin = %d (want 2)", scoped.Running, full.Running)
	}
}
