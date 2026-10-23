package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func spendFake(requests, bodies *[]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*requests = append(*requests, r.Method+" "+r.URL.RequestURI())
		*bodies = append(*bodies, string(body))
		switch {
		case r.URL.Path == "/api/v1/costs/app-caps":
			_, _ = w.Write([]byte(`{"caps":[{"app":"web","pct":50,"guard_mode":"block_deploys","condition":{"reason":"WithinBudget"},"forecast":{"pct":120}}]}`))
		case r.URL.Path == "/api/v1/costs/budgets" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"currency":"USD","budgets":[{"id":"cb_1","name":"spend cap: acme","scope":"project","scope_value":"proj_1","monthly":100,"guard_mode":"alert","action":"none","pct":40,"forecast_pct":90,"forecast":{"mid":90},"spent":40}]}`))
		case r.URL.Path == "/api/v1/costs/budgets":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"cb_2","name":"spend cap: other","monthly":50,"guard_mode":"block_deploys","action":"none"}`))
		case r.URL.Path == "/api/v1/costs/budgets/cb_1" && r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"id":"cb_1","monthly":100,"guard_mode":"block_deploys","action":"none"}`))
		case r.URL.Path == "/api/v1/costs/budgets/cb_1" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/v1/costs/budgets/cb_1/dry-run", r.URL.Path == "/api/v1/apps/web/budget/dry-run":
			_, _ = w.Write([]byte(`{"dry_run":true,"note":"Preview only.","apps":[{"name":"web","replicas":2,"daily_run_rate":3,"estimated_saving":30}],"total_replicas":2,"estimated_saving":30,"remaining_days":10,"currency":"USD"}`))
		case r.URL.Path == "/api/v1/apps/web/budget/override":
			_, _ = w.Write([]byte(`{"blocked":true,"allowed":true,"override":{"reason":"hotfix","expires_at":"2026-10-22T13:00:00Z"}}`))
		case r.URL.Path == "/api/v1/apps/web/budget" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"app":"web","period":"2026-10","resets_at":"2026-11-01T00:00:00Z","monthly_cost_ceiling":10,"action":"none","guard_mode":"alert","thresholds":[80,100],"condition":{"reason":"WithinBudget","message":"ok"},"forecast":{"pct":130,"cost":{"mid":13,"confidence":"low"}},"guard":{"blocked":true,"allowed":false}}`))
		case r.URL.Path == "/api/v1/apps/web/budget" && r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"app":"web","period":"2026-10","resets_at":"2026-11-01T00:00:00Z","condition":{"reason":"WithinBudget"}}`))
		default:
			http.NotFound(w, r)
		}
	})
}

func TestRun_SpendCaps(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantReq  []string
		wantBody string
	}{
		{"list", []string{"spend", "caps", "list"}, "web", []string{"GET /api/v1/costs/app-caps", "GET /api/v1/costs/budgets"}, ""},
		{"app status shows forecast and guard", []string{"spend", "caps", "status", "app", "web"}, "blocked", []string{"GET /api/v1/apps/web/budget"}, ""},
		{"project status", []string{"spend", "caps", "status", "project", "acme"}, "projected 90.00", []string{"GET /api/v1/costs/budgets"}, ""},
		{"app dry-run", []string{"spend", "caps", "dry-run", "app", "web"}, "2 replicas, about 30.00 USD", []string{"POST /api/v1/apps/web/budget/dry-run"}, ""},
		{"project dry-run", []string{"spend", "caps", "dry-run", "project", "acme"}, "Preview only.", []string{"GET /api/v1/costs/budgets", "POST /api/v1/costs/budgets/cb_1/dry-run"}, ""},
		{"app set keeps the rest", []string{"spend", "caps", "set", "app", "web", "--mode", "block_deploys", "--warn", "80,100"}, "web:", []string{"GET /api/v1/apps/web/budget", "PUT /api/v1/apps/web/budget"}, `"guard_mode":"block_deploys"`},
		{"project set updates the existing cap", []string{"spend", "caps", "set", "project", "acme", "--mode", "block_deploys"}, "guard block_deploys", []string{"GET /api/v1/costs/budgets", "PUT /api/v1/costs/budgets/cb_1"}, `"guard_mode":"block_deploys"`},
		{"project set creates a cap", []string{"spend", "caps", "set", "project", "other", "--cost", "50", "--mode", "block_deploys"}, "project other", []string{"GET /api/v1/costs/budgets", "POST /api/v1/costs/budgets"}, `"scope":"project"`},
		{"project rm", []string{"spend", "caps", "rm", "project", "acme"}, "removed the spend cap", []string{"GET /api/v1/costs/budgets", "DELETE /api/v1/costs/budgets/cb_1"}, ""},
		{"override", []string{"spend", "caps", "override", "app", "web", "--reason", "hotfix"}, "allowed until", []string{"POST /api/v1/apps/web/budget/override"}, `"reason":"hotfix"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var reqs, bodies []string
			srv := httptest.NewServer(spendFake(&reqs, &bodies))
			defer srv.Close()
			out, _ := runCLIExpectOK(t, append(tc.args, "--api-url", srv.URL))
			if !strings.Contains(out, tc.wantOut) {
				t.Fatalf("output %q missing %q", out, tc.wantOut)
			}
			if strings.Join(reqs, "|") != strings.Join(tc.wantReq, "|") {
				t.Fatalf("requests = %v, want %v", reqs, tc.wantReq)
			}
			if tc.wantBody != "" && !strings.Contains(strings.Join(bodies, ""), tc.wantBody) {
				t.Fatalf("bodies %q missing %q", bodies, tc.wantBody)
			}
		})
	}
}

func TestRun_SpendCapsUsageErrors(t *testing.T) {
	look := func(string) (string, bool) { return "", false }
	for name, args := range map[string][]string{
		"no args":               nil,
		"unknown verb":          {"caps", "explode"},
		"missing scope":         {"caps", "status", "web"},
		"override needs reason": {"caps", "override", "app", "web"},
		"override is app only":  {"caps", "override", "project", "acme", "--reason", "x"},
	} {
		var stdout, stderr strings.Builder
		if code := runSpend("cli", args, &stdout, &stderr, look); code != exitUsage {
			t.Errorf("%s = %d, want usage error", name, code)
		}
	}
}
