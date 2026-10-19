package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func costsFake(requests, bodies *[]string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*requests = append(*requests, r.Method+" "+r.URL.RequestURI())
		*bodies = append(*bodies, string(body))
		switch {
		case r.URL.Path == "/api/v1/costs/explore":
			_, _ = w.Write([]byte(`{"group_by":"app","granularity":"daily","currency":"USD","configured":true,
				"period":{"from":"2026-10-01T00:00:00Z","to":"2026-10-04T00:00:00Z"},"total":12,"previous":10,"delta_pct":20,
				"buckets":["2026-10-01"],"trend":[12],"series":[{"key":"web","total":12,"previous":10,"delta":2,"delta_pct":20,"values":[12]}],
				"movers":[{"key":"web","total":12,"previous":10,"delta":2,"delta_pct":20,"values":[12]}],
				"forecast":{"spent":12,"mid":90,"low":70,"high":110,"confidence":"medium","samples":3}}`))
		case r.URL.Path == "/api/v1/costs/budgets" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"currency":"USD","budgets":[{"id":"cb_1","name":"prod","scope":"tag","scope_value":"prod","monthly":100,"action":"pause","spent":85,"pct":85,"forecast":{"mid":120},"level":"warn","enforced":""}]}`))
		case r.URL.Path == "/api/v1/costs/budgets":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"cb_1","name":"prod"}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/costs/budgets/") && r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"id":"cb_1","name":"prod"}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/costs/budgets/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func TestRun_Costs(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantReq  string
		wantBody string
	}{
		{"explore", []string{"costs", "explore", "--by", "tag", "--period", "30d"}, "Forecast month end: 90.00 USD (range 70.00 to 110.00, medium confidence)", "GET /api/v1/costs/explore?granularity=daily&group_by=tag&period=30d", ""},
		{"explore movers", []string{"costs", "explore"}, "web  +2.00", "GET /api/v1/costs/explore?granularity=daily&group_by=app&period=mtd", ""},
		{"budgets list", []string{"costs", "budgets", "list"}, "cb_1", "GET /api/v1/costs/budgets", ""},
		{"budgets create", []string{"costs", "budgets", "create", "--name", "prod", "--scope", "tag", "--value", "prod", "--monthly", "100", "--thresholds", "50,90", "--action", "pause"}, "created budget cb_1", "POST /api/v1/costs/budgets", `"thresholds":[50,90]`},
		{"budgets set", []string{"costs", "budgets", "set", "cb_1", "--monthly", "150"}, "updated budget cb_1", "PUT /api/v1/costs/budgets/cb_1", `"monthly":150`},
		{"budgets delete", []string{"costs", "budgets", "delete", "cb_1"}, "deleted budget cb_1", "DELETE /api/v1/costs/budgets/cb_1", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var reqs, bodies []string
			srv := httptest.NewServer(costsFake(&reqs, &bodies))
			defer srv.Close()
			out, _ := runCLIExpectOK(t, append(tc.args, "--api-url", srv.URL))
			if !strings.Contains(out, tc.wantOut) {
				t.Fatalf("output %q missing %q", out, tc.wantOut)
			}
			if len(reqs) != 1 || reqs[0] != tc.wantReq {
				t.Fatalf("requests = %v, want %s", reqs, tc.wantReq)
			}
			if tc.wantBody != "" && !strings.Contains(bodies[0], tc.wantBody) {
				t.Fatalf("body %q missing %q", bodies[0], tc.wantBody)
			}
		})
	}
}

func TestRun_CostsUsageErrors(t *testing.T) {
	look := func(string) (string, bool) { return "", false }
	var stdout, stderr strings.Builder
	if code := runCosts("cli", nil, &stdout, &stderr, look); code != exitUsage {
		t.Fatalf("no args = %d", code)
	}
	if code := runCostsBudgets("cli", []string{"create", "--name", "x"}, &stdout, &stderr, look); code != exitUsage {
		t.Fatalf("create without scope = %d", code)
	}
	if code := runCostsBudgets("cli", []string{"set", "cb_1"}, &stdout, &stderr, look); code != exitUsage {
		t.Fatalf("set without changes = %d", code)
	}
}
