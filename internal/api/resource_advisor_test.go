package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

const testMiB = 1024 * 1024

// seedAdvisorScreenshot mirrors the reported server: six unlimited Postgres
// databases of 48 to 217 MiB and one app, three days of hourly samples.
func seedAdvisorScreenshot(t *testing.T, db *store.DB, tdb *telemetry.DB) {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "web", Image: "img:v1", Port: 3000}); err != nil {
		t.Fatal(err)
	}
	usage := map[string]float64{"service:web": 180}
	for name, mib := range map[string]float64{"askverdict": 217, "glinr": 126, "geokit": 71, "glinrv2": 68, "saasfolk": 67, "n8n": 48} {
		version := "16"
		if name == "askverdict" {
			version = "17-pgvector"
		}
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: name, Engine: store.EnginePostgres, Version: version}); err != nil {
			t.Fatal(err)
		}
		usage["database:"+name] = mib
	}
	now := time.Now().UTC().Truncate(time.Second)
	var samples []telemetry.Sample
	for id, mib := range usage {
		for i := 0; i < 72; i++ {
			ts := now.Add(-time.Duration(i) * time.Hour)
			jitter := 1 + float64(i%5)/100
			samples = append(samples,
				telemetry.Sample{ResourceID: id, Metric: "memory_usage_bytes", Timestamp: ts, Value: mib * testMiB * jitter},
				telemetry.Sample{ResourceID: id, Metric: "cpu_percent", Timestamp: ts, Value: 3 * jitter},
			)
		}
	}
	if err := tdb.WriteSamples(ctx, samples); err != nil {
		t.Fatal(err)
	}
}

func getAdvisorFindings(t *testing.T, rt *Router, cookie *http.Cookie, query string) []advisorFindingResource {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/resource-advisor/findings"+query, ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("findings status = %d: %s", rec.Code, rec.Body.String())
	}
	var resp advisorFindingsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp.Findings
}

func advisorPost(t *testing.T, rt *Router, cookie *http.Cookie, id, verb, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/resource-advisor/findings/"+url.PathEscape(id)+"/"+verb, body))
	return rec
}

func TestResourceAdvisor_OffByFlag(t *testing.T) {
	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	for _, path := range []string{"/api/v1/resource-advisor/findings", "/api/v1/resource-advisor/summary"} {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, path, ""))
		if rec.Code != http.StatusNotFound || !json.Valid(rec.Body.Bytes()) {
			t.Fatalf("%s = %d, want 404", path, rec.Code)
		}
	}
	if items := rt.resourceAdviceAttentionItems(httptest.NewRequest(http.MethodGet, "/", nil)); items != nil {
		t.Fatalf("attention items with flag off: %v", items)
	}
}

// fixedHostMemory pins a roomy 16 GiB host at 25% use so findings never
// depend on the machine running the tests.
func fixedHostMemory() (int64, int64, error) { return 16 << 30, 12 << 30, nil }

func TestResourceAdvisor_ScreenshotScenario(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	rt.hostMemory = fixedHostMemory
	cookie := loginTestSession(t, rt, db)
	seedAdvisorScreenshot(t, db, tdb)

	count := map[resourceadvisor.Kind]int{}
	for _, f := range getAdvisorFindings(t, rt, cookie, "") {
		count[f.Kind]++
		if len(f.Evidence.Series) != 0 {
			t.Fatalf("unfiltered list should omit series, %s has %d", f.ID, len(f.Evidence.Series))
		}
	}
	if count[resourceadvisor.KindMemoryLimitMissing] != 7 || count[resourceadvisor.KindEngineTuning] != 1 {
		t.Fatalf("findings = %v", count)
	}
	one := getAdvisorFindings(t, rt, cookie, "?resource=database:askverdict")
	if len(one) == 0 {
		t.Fatal("no findings for askverdict")
	}
	for _, f := range one {
		if f.Resource.Name != "askverdict" {
			t.Fatalf("filter leaked %s", f.ID)
		}
		if f.Kind == resourceadvisor.KindMemoryLimitMissing && (len(f.Evidence.Series) == 0 || !f.CanApply) {
			t.Fatalf("limit finding should carry a series and be appliable: %+v", f)
		}
	}
	if warn := getAdvisorFindings(t, rt, cookie, "?severity=warn"); len(warn) != 0 {
		t.Fatalf("no warnings expected, got %d", len(warn))
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/resource-advisor/findings?resource=pod:x", ""))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad resource filter = %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/resource-advisor/summary", ""))
	var sum advisorSummaryResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &sum); err != nil || sum.Actionable < 7 {
		t.Fatalf("summary = %+v, %v", sum, err)
	}
}

func TestResourceAdvisor_ApplyPreviewThenConfirm(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedAdvisorScreenshot(t, db, tdb)
	id := "memory_limit_missing:database:askverdict"
	ctx := context.Background()

	rec := advisorPost(t, rt, cookie, id, "apply", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d: %s", rec.Code, rec.Body.String())
	}
	var prev advisorApplyResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &prev)
	if !prev.ConfirmRequired || prev.Applied || prev.Before.MemoryBytes != 0 || prev.After.MemoryBytes < 256*testMiB {
		t.Fatalf("preview = %+v", prev)
	}
	if d, _ := db.GetDesiredDatabase(ctx, "askverdict"); d.Resources != nil && d.Resources.MemoryBytes != 0 {
		t.Fatal("preview changed the database")
	}

	rec = advisorPost(t, rt, cookie, id, "apply", `{"confirm":true}`)
	var done advisorApplyResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &done)
	if rec.Code != http.StatusOK || !done.Applied {
		t.Fatalf("confirm = %d %+v", rec.Code, done)
	}
	d, err := db.GetDesiredDatabase(ctx, "askverdict")
	if err != nil || d.Resources == nil || d.Resources.MemoryBytes != prev.After.MemoryBytes {
		t.Fatalf("limit not saved: %+v %v", d.Resources, err)
	}
	if again := advisorPost(t, rt, cookie, id, "apply", `{"confirm":true}`); again.Code != http.StatusNotFound {
		t.Fatalf("finding should be gone after apply, got %d", again.Code)
	}
	if bad := advisorPost(t, rt, cookie, "engine_tuning:database:askverdict", "apply", `{"confirm":true}`); bad.Code != http.StatusConflict {
		t.Fatalf("tuning apply = %d, want 409", bad.Code)
	}
}

func TestResourceAdvisor_ApplyRefusesStaleValue(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedAdvisorScreenshot(t, db, tdb)
	if rec := advisorPost(t, rt, cookie, "memory_limit_missing:app:web", "apply", ""); rec.Code != http.StatusOK {
		t.Fatalf("preview = %d", rec.Code)
	}
	ctx := context.Background()
	svc, _ := db.GetDesiredService(ctx, "web")
	svc.Resources = &store.ServiceResources{MemoryBytes: 300 * testMiB}
	if err := db.SaveDesiredService(ctx, *svc); err != nil {
		t.Fatal(err)
	}
	if rec := advisorPost(t, rt, cookie, "memory_limit_missing:app:web", "apply", `{"confirm":true}`); rec.Code != http.StatusConflict {
		t.Fatalf("stale apply = %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if got, _ := db.GetDesiredService(ctx, "web"); got.Resources.MemoryBytes != 300*testMiB {
		t.Fatal("stale apply overwrote the operator's change")
	}
}

func TestResourceAdvisor_DismissSnoozeRestore(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	cookie := loginTestSession(t, rt, db)
	seedAdvisorScreenshot(t, db, tdb)
	id := "memory_limit_missing:database:n8n"
	has := func(fs []advisorFindingResource) (advisorFindingResource, bool) {
		for _, f := range fs {
			if f.ID == id {
				return f, true
			}
		}
		return advisorFindingResource{}, false
	}
	if rec := advisorPost(t, rt, cookie, id, "dismiss", ""); rec.Code != http.StatusOK {
		t.Fatalf("dismiss = %d", rec.Code)
	}
	if _, ok := has(getAdvisorFindings(t, rt, cookie, "")); ok {
		t.Fatal("dismissed finding still listed")
	}
	if f, ok := has(getAdvisorFindings(t, rt, cookie, "?include=hidden")); !ok || f.State != store.AdvisorFindingDismissed {
		t.Fatalf("include=hidden = %+v", f)
	}
	if rec := advisorPost(t, rt, cookie, id, "snooze", `{"duration":"365d"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("long snooze = %d", rec.Code)
	}
	if rec := advisorPost(t, rt, cookie, id, "snooze", `{"duration":"2d"}`); rec.Code != http.StatusOK {
		t.Fatalf("snooze = %d", rec.Code)
	}
	if f, ok := has(getAdvisorFindings(t, rt, cookie, "?include=hidden")); !ok || f.State != store.AdvisorFindingSnoozed || f.SnoozedUntil == "" {
		t.Fatalf("snoozed = %+v", f)
	}
	if rec := advisorPost(t, rt, cookie, id, "restore", ""); rec.Code != http.StatusOK {
		t.Fatalf("restore = %d", rec.Code)
	}
	if _, ok := has(getAdvisorFindings(t, rt, cookie, "")); !ok {
		t.Fatal("restored finding missing")
	}
	if rec := advisorPost(t, rt, cookie, "nope:app:web", "dismiss", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id = %d", rec.Code)
	}
}

func TestResourceAdvisor_ReadOnlyTokenCannotApply(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	seedAdvisorScreenshot(t, db, tdb)
	token := seedMatrixToken(t, db, "advisor-ro", []string{AbilityRead})
	get := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource-advisor/findings", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rt.Handler().ServeHTTP(get, req)
	if get.Code != http.StatusOK {
		t.Fatalf("read-only GET = %d", get.Code)
	}
	var resp advisorFindingsResponse
	_ = json.Unmarshal(get.Body.Bytes(), &resp)
	for _, f := range resp.Findings {
		if f.CanApply {
			t.Fatalf("read-only caller offered apply on %s", f.ID)
		}
		if f.Resource.Kind == resourceadvisor.ResourceNode {
			t.Fatalf("node finding %s visible without root", f.ID)
		}
	}
	for _, verb := range []string{"apply", "dismiss", "snooze", "restore"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/resource-advisor/findings/"+url.PathEscape("memory_limit_missing:app:web")+"/"+verb, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("read-only %s = %d, want 403", verb, rec.Code)
		}
	}
}

func TestResourceAdvisor_TeamScope(t *testing.T) {
	f := newTeamFixture(t)
	for _, role := range allTeamRoles {
		fs := getAdvisorFindings(t, f.rt, f.cookies[role], "")
		if len(fs) == 0 {
			t.Fatalf("%s sees no findings for its own team", role)
		}
		for _, fd := range fs {
			if fd.Resource.Name == appB || fd.Resource.Name == dbB || fd.Resource.Kind == resourceadvisor.ResourceNode {
				t.Fatalf("%s of team A sees %s", role, fd.ID)
			}
		}
		if rec := advisorPost(t, f.rt, f.cookies[role], "not_enough_data:app:"+appB, "dismiss", ""); rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
			t.Fatalf("%s dismissed team B finding: %d", role, rec.Code)
		}
	}
	if rec := advisorPost(t, f.rt, f.cookies[store.TeamRoleViewer], "not_enough_data:app:"+appA, "dismiss", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer dismissed own team finding: %d", rec.Code)
	}
}

func TestBetaFeatures_ToggleFromSettings(t *testing.T) {
	experimental.Set()
	t.Cleanup(func() { experimental.Set(experimental.All()...) })
	rt, db := newTestRouter(t)
	cookie := loginTestSession(t, rt, db)
	put := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/beta-features/resource-advisor", body))
		return rec
	}
	if rec := put(`{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled = %d", rec.Code)
	}
	if rec := put(`{"enabled":true}`); rec.Code != http.StatusOK || !experimental.Enabled(experimental.ResourceAdvisor) {
		t.Fatalf("enable = %d", rec.Code)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/resource-advisor/summary", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("advisor after enabling = %d", rec.Code)
	}
	experimental.Set()
	if err := LoadBetaFeatures(context.Background(), db); err != nil || !experimental.Enabled(experimental.ResourceAdvisor) {
		t.Fatalf("stored switch not reloaded: %v", err)
	}
	if rec := put(`{"enabled":false}`); rec.Code != http.StatusOK || experimental.Enabled(experimental.ResourceAdvisor) {
		t.Fatalf("disable = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings/beta-features/iac", nil)
	req.AddCookie(cookie)
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPut, "/api/v1/settings/beta-features/iac", `{"enabled":true}`))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"pending_restart":true`) || experimental.Enabled(experimental.IaC) {
		t.Fatalf("startup wired feature = %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/settings/beta-features", ""))
	var list []betaFeatureResource
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != len(experimental.All()) {
		t.Fatalf("list = %s", rec.Body.String())
	}
	for _, f := range list {
		if f.EnabledBy != betaSourceOff {
			t.Fatalf("%s source = %s, want off", f.ID, f.EnabledBy)
		}
	}
}

func TestResourceAdvisor_AttentionItems(t *testing.T) {
	rt, db, tdb := newTestRouterWithTelemetry(t)
	rt.hostMemory = fixedHostMemory
	cookie := loginTestSession(t, rt, db)
	seedAdvisorScreenshot(t, db, tdb)
	if _, err := rt.advisorFindings(context.Background()); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/attention/feed", ""))
	var feed attentionFeedResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &feed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, it := range feed.Items {
		if it.Kind == "resource_advice" {
			found = true
			if it.Class != "recommendation" || it.Params["count"] == "" {
				t.Fatalf("advice item = %+v", it)
			}
		}
	}
	if !found {
		t.Fatal("no resource_advice item in the attention feed")
	}
}

func TestDatabaseLinks(t *testing.T) {
	dbs := []store.DesiredDatabase{{Name: "main"}, {Name: "spare"}}
	tests := []struct {
		name string
		svcs []store.DesiredService
		want map[string]int
	}{
		{"managed ref", []store.DesiredService{{Name: "a", DatabaseEnv: map[string]store.DatabaseEnvRef{"DATABASE_URL": {Database: "main"}}}}, map[string]int{"main": 1, "spare": 0}},
		{"attachment", []store.DesiredService{{Name: "a", DatabaseAttachment: &store.DatabaseAttachment{DatabaseName: "spare"}}}, map[string]int{"main": 0, "spare": 1}},
		{"plain env names the host", []store.DesiredService{{Name: "a", Env: map[string]string{"DB": "postgres://u@" + databaseContainerName("main") + ":5432/x"}}}, map[string]int{"main": 1, "spare": 0}},
		{"unreadable secret makes it unknown", []store.DesiredService{{Name: "a", SecretEnv: []store.SecretEnvRef{{Name: "DATABASE_URL"}}}}, map[string]int{"main": -1, "spare": -1}},
	}
	for _, tc := range tests {
		got := databaseLinks(tc.svcs, dbs)
		for k, v := range tc.want {
			if got[k] != v {
				t.Errorf("%s: %s = %d, want %d", tc.name, k, got[k], v)
			}
		}
	}
}

func TestResourceAdvisor_HostMemoryThreshold(t *testing.T) {
	const gib = int64(1) << 30
	tests := []struct {
		name      string
		total     int64
		available int64
		err       error
		wantWarn  int
	}{
		{"well under the pressure line", 16 * gib, 12 * gib, nil, 0},
		{"just under 90 percent", 100 * gib, 11 * gib, nil, 0},
		{"at 90 percent", 100 * gib, 10 * gib, nil, 1},
		{"nearly exhausted", 36 * gib, 3 * gib, nil, 1},
		{"unreadable host memory", 0, 0, errors.New("no meminfo"), 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, db, tdb := newTestRouterWithTelemetry(t)
			rt.hostMemory = func() (int64, int64, error) { return tt.total, tt.available, tt.err }
			cookie := loginTestSession(t, rt, db)
			seedAdvisorScreenshot(t, db, tdb)
			warns := 0
			for _, f := range getAdvisorFindings(t, rt, cookie, "?severity=warn") {
				if f.Kind == resourceadvisor.KindHostMemoryPressure {
					warns++
				}
			}
			if warns != tt.wantWarn {
				t.Fatalf("host pressure findings = %d, want %d", warns, tt.wantWarn)
			}
		})
	}
}
