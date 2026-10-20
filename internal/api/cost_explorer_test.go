package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/spendcap"
)

func skipEarlyInMonth(t *testing.T) {
	t.Helper()
	if time.Now().UTC().Day() <= usageSeedDays {
		t.Skip("seeded days fall in the previous month")
	}
}

func TestCostExplore_GroupsAndValidates(t *testing.T) {
	f := newUsageFixture(t)
	f.configureNode(t, 0)
	var resp costExploreResponse
	rec := f.do(t, f.admin, http.MethodGet, "/api/v1/costs/explore?period=30d&group_by=app", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("explore = %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Series) != 2 || len(resp.Buckets) != 30 || resp.Total <= 0 || resp.Forecast == nil {
		t.Fatalf("series = %d buckets = %d total = %v forecast = %v", len(resp.Series), len(resp.Buckets), resp.Total, resp.Forecast)
	}
	rec = f.do(t, f.admin, http.MethodGet, "/api/v1/costs/explore?granularity=monthly&months=3&group_by=project", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("monthly = %d: %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || len(resp.Buckets) != 3 {
		t.Fatalf("monthly buckets = %v err = %v", resp.Buckets, err)
	}
	for _, q := range []string{"group_by=team", "granularity=hourly", "granularity=monthly&months=99"} {
		if rec := f.do(t, f.admin, http.MethodGet, "/api/v1/costs/explore?"+q, ""); rec.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", q, rec.Code)
		}
	}
}

func TestCostExplore_TenantOnlySeesOwnTeam(t *testing.T) {
	f := newUsageFixture(t)
	f.configureNode(t, 0)
	rec := f.do(t, f.viewer, http.MethodGet, "/api/v1/costs/explore?period=30d&group_by=app", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("viewer explore = %d: %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), usageAppB) {
		t.Fatalf("viewer saw another team's app: %s", rec.Body.String())
	}
	if rec := f.do(t, f.viewer, http.MethodGet, "/api/v1/costs/budgets", ""); rec.Code != http.StatusForbidden {
		t.Fatalf("viewer budgets = %d, want 403", rec.Code)
	}
}

func TestCostBudgets_BreachAlertsAndEnforcesThenLifts(t *testing.T) {
	skipEarlyInMonth(t)
	f := newUsageFixture(t)
	f.configureNode(t, 0)
	ctx := context.Background()
	body := jsonBody(t, map[string]any{"name": "team a", "scope": "project", "scope_value": usageProjA, "monthly": 0.01, "action": "pause"})
	rec := f.do(t, f.admin, http.MethodPost, "/api/v1/costs/budgets", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("created = %s", rec.Body.String())
	}
	if dup := f.do(t, f.admin, http.MethodPost, "/api/v1/costs/budgets", body); dup.Code != http.StatusConflict {
		t.Fatalf("duplicate name = %d, want 409", dup.Code)
	}

	var list costBudgetsResponse
	rec = f.do(t, f.admin, http.MethodGet, "/api/v1/costs/budgets", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list.Budgets) != 1 {
		t.Fatalf("list = %s", rec.Body.String())
	}
	b := list.Budgets[0]
	if b.Level != costLevelOver || b.Enforced != spendcap.ActionPause || len(b.Apps) != 1 || b.Apps[0] != usageAppA || b.SuggestedMonthly <= b.Monthly {
		t.Fatalf("status = %+v", b)
	}

	enf, err := f.db.ListSpendCapEnforcements(ctx, spendcap.Period(time.Now()))
	if err != nil || len(enf) != 1 || enf[0].App != usageAppA || enf[0].Action != spendcap.ActionPause {
		t.Fatalf("enforcements = %+v err = %v", enf, err)
	}

	rule := alerting.Rule{ID: "r1", Kind: alerting.KindBudgetThreshold, Name: "budgets", Enabled: true}
	next, notices, keys, err := alerting.EvaluateBudgetThreshold(ctx, f.rt, rule, time.Now())
	if err != nil || !next.Firing || len(keys) != 1 || !strings.Contains(keys[0], "costbudget:"+created.ID) || !strings.Contains(notices[0], "paused") {
		t.Fatalf("firing = %v notices = %v keys = %v err = %v", next.Firing, notices, keys, err)
	}

	raise := jsonBody(t, map[string]any{"monthly": 1000000})
	if rec := f.do(t, f.admin, http.MethodPut, "/api/v1/costs/budgets/"+created.ID, raise); rec.Code != http.StatusOK {
		t.Fatalf("raise = %d: %s", rec.Code, rec.Body.String())
	}
	enf, err = f.db.ListSpendCapEnforcements(ctx, spendcap.Period(time.Now()))
	if err != nil || len(enf) != 0 {
		t.Fatalf("raising the budget should lift enforcement: %+v err = %v", enf, err)
	}

	if rec := f.do(t, f.admin, http.MethodDelete, "/api/v1/costs/budgets/"+created.ID, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := f.do(t, f.admin, http.MethodDelete, "/api/v1/costs/budgets/"+created.ID, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", rec.Code)
	}
}

func TestCostBudgets_RejectsBadInput(t *testing.T) {
	f := newUsageFixture(t)
	cases := []map[string]any{
		{"name": "x", "scope": "app", "scope_value": "no-such-app", "monthly": 5},
		{"name": "x", "scope": "project", "scope_value": "no-such-project", "monthly": 5},
		{"name": "x", "scope": "tag", "scope_value": "prod", "monthly": 0},
		{"name": "x", "scope": "tag", "scope_value": "prod", "monthly": 5, "action": "delete"},
	}
	for i, c := range cases {
		if rec := f.do(t, f.admin, http.MethodPost, "/api/v1/costs/budgets", jsonBody(t, c)); rec.Code != http.StatusBadRequest {
			t.Errorf("case %d = %d, want 400: %s", i, rec.Code, rec.Body.String())
		}
	}
}
