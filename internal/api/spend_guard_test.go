package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spendcap"
)

func TestSpendGuard_ProjectBudgetBlocksThenOverrideAllows(t *testing.T) {
	skipEarlyInMonth(t)
	f := newUsageFixture(t)
	f.configureNode(t, 0)
	body := jsonBody(t, map[string]any{"name": "team a", "scope": "project", "scope_value": usageProjA, "monthly": 0.01, "guard_mode": "block_deploys"})
	if rec := f.do(t, f.admin, http.MethodPost, "/api/v1/costs/budgets", body); rec.Code != http.StatusCreated {
		t.Fatalf("create = %d: %s", rec.Code, rec.Body.String())
	}
	g := f.rt.spendGuard()
	if g == nil {
		t.Fatal("guard is not wired")
	}
	st, err := g.Check(t.Context(), usageAppA)
	if err != nil || !st.Blocked || st.Allowed {
		t.Fatalf("over budget must block: %+v err=%v", st, err)
	}
	if st, _ := g.Check(t.Context(), usageAppB); st.Blocked {
		t.Fatalf("an app outside the project is not blocked: %+v", st)
	}

	path := "/api/v1/apps/" + usageAppA + "/budget/override"
	if rec := f.do(t, f.admin, http.MethodPost, path, `{"reason":"  "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("override without a reason = %d", rec.Code)
	}
	if rec := f.do(t, f.viewer, http.MethodPost, path, `{"reason":"let me in"}`); rec.Code != http.StatusForbidden {
		t.Fatalf("override by a non-root = %d, want 403", rec.Code)
	}
	rec := f.do(t, f.admin, http.MethodPost, path, `{"reason":"checkout hotfix"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("override = %d: %s", rec.Code, rec.Body.String())
	}
	if st, _ := g.Check(t.Context(), usageAppA); !st.Allowed || st.Override == nil || st.Override.Reason != "checkout hotfix" {
		t.Fatalf("an override must allow the deploy: %+v", st)
	}
	hist, err := f.db.ListSpendCapEvents(t.Context(), usageAppA, 10)
	if err != nil || len(hist) != 1 || hist[0].Kind != spendcap.EventOverride {
		t.Fatalf("the override must be audited in history: %+v err=%v", hist, err)
	}
}

func TestSpendGuard_DryRunHasNoSideEffects(t *testing.T) {
	skipEarlyInMonth(t)
	f := newUsageFixture(t)
	f.configureNode(t, 0)
	body := jsonBody(t, map[string]any{"name": "team a", "scope": "project", "scope_value": usageProjA, "monthly": 1000000, "action": "pause"})
	rec := f.do(t, f.admin, http.MethodPost, "/api/v1/costs/budgets", body)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("created = %s", rec.Body.String())
	}
	rec = f.do(t, f.viewer, http.MethodPost, "/api/v1/costs/budgets/"+created.ID+"/dry-run", "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a viewer without cost access = %d, want 403", rec.Code)
	}
	rec = f.do(t, f.admin, http.MethodPost, "/api/v1/costs/budgets/"+created.ID+"/dry-run", "")
	var res dryRunResource
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &res) != nil {
		t.Fatalf("dry-run = %d: %s", rec.Code, rec.Body.String())
	}
	if !res.DryRun || len(res.Apps) != 1 || res.Apps[0].Name != usageAppA || res.Apps[0].EstimatedSaving < 0 || res.Note == "" {
		t.Fatalf("preview = %+v", res)
	}
	enf, err := f.db.ListSpendCapEnforcements(t.Context(), spendcap.Period(time.Now()))
	if err != nil || len(enf) != 0 {
		t.Fatalf("a dry-run must not enforce anything: %+v err=%v", enf, err)
	}
	if rec := f.do(t, f.admin, http.MethodPost, "/api/v1/apps/"+usageAppA+"/budget/dry-run", ""); rec.Code != http.StatusOK {
		t.Fatalf("app dry-run = %d: %s", rec.Code, rec.Body.String())
	}
	if rec := f.do(t, f.admin, http.MethodPost, "/api/v1/apps/ghost/budget/dry-run", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown app dry-run = %d", rec.Code)
	}
}

func TestSpendGuard_AppCapKeepsGuardWhenOmitted(t *testing.T) {
	f := newUsageFixture(t)
	put := func(body string) *http.Response {
		rec := f.do(t, f.admin, http.MethodPut, "/api/v1/apps/"+usageAppA+"/budget", body)
		return rec.Result()
	}
	if res := put(`{"monthly_cost_ceiling":50,"guard_mode":"block_deploys"}`); res.StatusCode != http.StatusOK {
		t.Fatalf("set guard = %d", res.StatusCode)
	}
	if res := put(`{"monthly_cost_ceiling":60}`); res.StatusCode != http.StatusOK {
		t.Fatalf("update = %d", res.StatusCode)
	}
	cp, err := f.db.GetSpendCap(t.Context(), usageAppA)
	if err != nil || cp.GuardMode != spendcap.GuardBlockDeploys || cp.MonthlyCostCeiling != 60 {
		t.Fatalf("an update that omits guard_mode must keep it: %+v err=%v", cp, err)
	}
	if res := put(`{"monthly_cost_ceiling":60,"guard_mode":"stop"}`); res.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad guard mode = %d", res.StatusCode)
	}
	if res := put(`{"monthly_cost_ceiling":60,"guard_mode":"alert"}`); res.StatusCode != http.StatusOK {
		t.Fatalf("lift guard = %d", res.StatusCode)
	}
}
