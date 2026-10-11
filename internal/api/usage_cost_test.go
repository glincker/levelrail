package api

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
	"github.com/GLINCKER/levelrail/internal/usagecost"
)

const (
	usageTeamA, usageTeamB = "org_usage_a", "org_usage_b"
	usageProjA, usageProjB = "proj_usage_a", "proj_usage_b"
	usageAppA, usageAppB   = "usage-a", "usage-b"
	usagePassword          = "usage-test-password-1"
	usageSeedDays          = 3
	usageNodePrice         = 310.0
)

type usageFixture struct {
	rt     *Router
	db     *store.DB
	admin  *http.Cookie
	viewer *http.Cookie
	adminA *http.Cookie
}

// newUsageFixture seeds two projects of two teams, one app each using one
// core and 2 GiB for each of the last three UTC days, on one 4 vCPU 8 GiB
// node billed 310 a month.
func newUsageFixture(t *testing.T) *usageFixture {
	t.Helper()
	db := openTestDB(t)
	tdb := newTestTelemetryDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, WithTelemetryQuerier(telemetry.NewLocalFederator(tdb)))
	admin := loginTestSession(t, rt, db)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, s := range []struct{ team, proj, app string }{{usageTeamA, usageProjA, usageAppA}, {usageTeamB, usageProjB, usageAppB}} {
		must(db.SaveOrganization(ctx, store.Organization{ID: s.team, Name: s.team + " name", CreatedAt: "2026-10-01T00:00:00Z"}))
		must(db.SaveProject(ctx, store.Project{ID: s.proj, Name: s.proj + " name", CreatedAt: "2026-10-01T00:00:00Z"}))
		must(db.SetProjectOrganization(ctx, s.proj, s.team))
		must(db.SaveDesiredService(ctx, store.DesiredService{Name: s.app, Image: "nginx:1", Port: 80}))
		must(db.UpdateServiceProject(ctx, s.app, s.proj))
	}
	today := usagecost.StartOfDay(time.Now())
	var samples []telemetry.Sample
	for _, app := range []string{usageAppA, usageAppB} {
		for d := 1; d <= usageSeedDays; d++ {
			start := today.AddDate(0, 0, -d)
			for m := 0; m < 24*60; m += 5 {
				ts := start.Add(time.Duration(m) * time.Minute)
				samples = append(samples,
					telemetry.Sample{ResourceID: "service:" + app, Metric: "cpu_percent", Timestamp: ts, Value: 100},
					telemetry.Sample{ResourceID: "service:" + app, Metric: "memory_usage_bytes", Timestamp: ts, Value: 2 * (1 << 30)},
				)
			}
		}
	}
	must(tdb.WriteSamples(ctx, samples))
	f := &usageFixture{rt: rt, db: db, admin: admin}
	viewer, err := rt.createLocalUserInTeam(ctx, "viewer@usage-a.test", "", usagePassword, store.TeamRoleAbilities(store.TeamRoleViewer), usageTeamA, store.TeamRoleViewer)
	must(err)
	f.viewer = sessionCookieForTest(t, rt, viewer.ID)
	adm, err := rt.createLocalUserInTeam(ctx, "admin@usage-a.test", "", usagePassword, store.TeamRoleAbilities(store.TeamRoleAdmin), usageTeamA, store.TeamRoleAdmin)
	must(err)
	f.adminA = sessionCookieForTest(t, rt, adm.ID)
	return f
}

func (f *usageFixture) do(t *testing.T, cookie *http.Cookie, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	f.rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, method, path, body))
	return rec
}

func (f *usageFixture) configureNode(t *testing.T, spreadPct float64) {
	t.Helper()
	body := jsonBody(t, map[string]any{
		"settings": map[string]any{
			"mode": "node", "currency": "USD", "basis": "avg", "cpu_weight_pct": 50, "idle_spread_pct": spreadPct,
		},
		"nodes": []map[string]any{{"node_id": "local", "monthly_price": usageNodePrice, "vcpus": 4, "memory_gib": 8}},
	})
	if rec := f.do(t, f.admin, http.MethodPut, "/api/v1/usage/settings", body); rec.Code != http.StatusOK {
		t.Fatalf("put settings = %d: %s", rec.Code, rec.Body.String())
	}
}

func expectedUsageShare() float64 {
	today := usagecost.StartOfDay(time.Now())
	var total float64
	for d := 1; d <= usageSeedDays; d++ {
		day := today.AddDate(0, 0, -d)
		total += usageNodePrice / float64(usagecost.DaysInMonth(day)) * 0.25
	}
	return total
}

func getUsageReport(t *testing.T, f *usageFixture, cookie *http.Cookie, query string) usageReportResponse {
	t.Helper()
	rec := f.do(t, cookie, http.MethodGet, "/api/v1/usage/report"+query, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("report = %d: %s", rec.Code, rec.Body.String())
	}
	var resp usageReportResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestUsageReport_AllocatesNodePriceByUsageAndExportsCSV(t *testing.T) {
	f := newUsageFixture(t)
	f.configureNode(t, 0)

	resp := getUsageReport(t, f, f.admin, "?period=30d")
	if !resp.Configured || resp.Mode != usagecost.ModeNode {
		t.Fatalf("configured = %v mode = %q", resp.Configured, resp.Mode)
	}
	want := expectedUsageShare()
	byProject := map[string]float64{}
	for _, g := range resp.Projects {
		byProject[g.Key] = g.Total
	}
	for _, p := range []string{usageProjA, usageProjB} {
		if math.Abs(byProject[p]-want) > want*0.01 {
			t.Errorf("project %s = %.4f, want about %.4f", p, byProject[p], want)
		}
	}
	if resp.Totals.Unallocated < want*2*0.99 {
		t.Errorf("unallocated = %.4f, want at least the %.4f idle half of the bill", resp.Totals.Unallocated, want*2)
	}

	rec := f.do(t, f.admin, http.MethodGet, "/api/v1/usage/report?period=30d&format=csv", "")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") {
		t.Fatalf("csv = %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	out := rec.Body.String()
	for _, needle := range []string{"resource_kind,resource,project", usageAppA, usageAppB, "unallocated"} {
		if !strings.Contains(out, needle) {
			t.Errorf("csv missing %q:\n%s", needle, out)
		}
	}
}

func TestUsageReport_IdleSpreadAllocatesWholeBill(t *testing.T) {
	f := newUsageFixture(t)
	f.configureNode(t, 100)
	resp := getUsageReport(t, f, f.admin, "?period=30d")
	var apps float64
	for _, g := range resp.Projects {
		apps += g.Total
	}
	want := expectedUsageShare() * 2 * 2
	if math.Abs(apps-want) > want*0.01 {
		t.Errorf("allocated = %.4f, want about %.4f with the idle half spread", apps, want)
	}
}

func TestUsageBudget_FiresThroughAlertRule(t *testing.T) {
	f := newUsageFixture(t)
	f.configureNode(t, 0)
	body := jsonBody(t, map[string]any{"scope": "project", "scope_id": usageProjA, "monthly": 0.01})
	if rec := f.do(t, f.admin, http.MethodPut, "/api/v1/usage/budgets", body); rec.Code != http.StatusOK {
		t.Fatalf("put budget = %d: %s", rec.Code, rec.Body.String())
	}
	rule := alerting.Rule{ID: "r1", Kind: alerting.KindBudgetThreshold, Name: "budgets", Enabled: true}
	next, notices, keys, err := alerting.EvaluateBudgetThreshold(context.Background(), f.rt, rule, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !next.Firing || len(notices) == 0 || !strings.Contains(notices[0], "over budget") {
		t.Fatalf("firing = %v notices = %v keys = %v", next.Firing, notices, keys)
	}
}

func TestUsageScope_TenantSeesOnlyOwnTeam(t *testing.T) {
	f := newUsageFixture(t)
	f.configureNode(t, 0)
	ctx := context.Background()
	for _, b := range []usagecost.Budget{
		{Scope: usagecost.ScopeProject, ScopeID: usageProjA, Monthly: 100},
		{Scope: usagecost.ScopeProject, ScopeID: usageProjB, Monthly: 100},
	} {
		if _, err := f.db.SaveUsageBudget(ctx, b, time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	resp := getUsageReport(t, f, f.viewer, "?period=30d")
	if resp.IncludesUnallocated || resp.Totals.Unallocated != 0 || len(resp.UnpricedNodes) != 0 {
		t.Errorf("tenant saw instance wide figures: %+v", resp.Totals)
	}
	for _, g := range resp.Resources {
		if g.Name == usageAppB {
			t.Errorf("tenant saw team B resource %s", g.Key)
		}
	}
	for k := range resp.Labels {
		if strings.Contains(k, usageProjB) || strings.Contains(k, usageTeamB) {
			t.Errorf("tenant saw label %q of another team", k)
		}
	}
	if len(resp.Resources) != 1 || resp.Resources[0].Name != usageAppA {
		t.Errorf("tenant resources = %+v", resp.Resources)
	}
	if len(resp.Teams) != 1 || resp.Teams[0].Key != usageTeamA {
		t.Errorf("tenant teams = %+v", resp.Teams)
	}

	rec := f.do(t, f.viewer, http.MethodGet, "/api/v1/usage/budgets", "")
	var budgets usageBudgetsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &budgets); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("budgets = %d %v", rec.Code, err)
	}
	if len(budgets.Budgets) != 1 || budgets.Budgets[0].ScopeID != usageProjA {
		t.Errorf("tenant budgets = %+v", budgets.Budgets)
	}

	if rec := f.do(t, f.viewer, http.MethodGet, "/api/v1/usage/settings", ""); rec.Code != http.StatusForbidden {
		t.Errorf("tenant settings = %d, want 403", rec.Code)
	}
	if rec := f.do(t, f.viewer, http.MethodPut, "/api/v1/usage/budgets", jsonBody(t, map[string]any{"scope": "project", "scope_id": usageProjA, "monthly": 5})); rec.Code != http.StatusForbidden {
		t.Errorf("viewer budget write = %d, want 403", rec.Code)
	}
	if rec := f.do(t, f.adminA, http.MethodPut, "/api/v1/usage/budgets", jsonBody(t, map[string]any{"scope": "project", "scope_id": usageProjB, "monthly": 5})); rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Errorf("team A admin writing team B budget = %d, want 403 or 404", rec.Code)
	}
	if rec := f.do(t, f.adminA, http.MethodPut, "/api/v1/usage/budgets", jsonBody(t, map[string]any{"scope": "project", "scope_id": usageProjA, "monthly": 50})); rec.Code != http.StatusOK {
		t.Errorf("team A admin writing own budget = %d: %s", rec.Code, rec.Body.String())
	}
	all, err := f.db.ListUsageBudgets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range all {
		if b.ScopeID == usageProjB && b.Monthly != 100 {
			t.Errorf("team B budget changed to %v", b.Monthly)
		}
		if b.ScopeID == usageProjB {
			if rec := f.do(t, f.adminA, http.MethodDelete, "/api/v1/usage/budgets/"+b.ID, ""); rec.Code != http.StatusNotFound && rec.Code != http.StatusForbidden {
				t.Errorf("deleting other team budget = %d", rec.Code)
			}
		}
	}
}

func TestUsageReport_RejectsBadPeriod(t *testing.T) {
	f := newUsageFixture(t)
	for _, q := range []string{"?period=forever", "?period=custom&from=2026-10-05&to=2026-10-01", "?period=custom&from=nope&to=2026-10-01"} {
		if rec := f.do(t, f.admin, http.MethodGet, "/api/v1/usage/report"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", q, rec.Code)
		}
	}
}

func TestUsageSettings_Validation(t *testing.T) {
	f := newUsageFixture(t)
	bad := []map[string]any{
		{"settings": map[string]any{"mode": "magic", "currency": "USD", "basis": "avg", "cpu_weight_pct": 50}},
		{"settings": map[string]any{"mode": "unit", "currency": "USD", "basis": "avg", "cpu_weight_pct": 50, "cpu_core_hour": -1}},
		{"settings": map[string]any{"mode": "unit", "currency": "USD", "basis": "avg", "cpu_weight_pct": 50, "idle_spread_pct": 150}},
		{"nodes": []map[string]any{{"node_id": "", "monthly_price": 5}}},
		{},
	}
	for i, b := range bad {
		if rec := f.do(t, f.admin, http.MethodPut, "/api/v1/usage/settings", jsonBody(t, b)); rec.Code != http.StatusBadRequest {
			t.Errorf("case %d = %d, want 400", i, rec.Code)
		}
	}
}
