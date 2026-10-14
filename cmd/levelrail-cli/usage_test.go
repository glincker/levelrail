package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type usageFake struct {
	requests []string
	bodies   []string
}

func (f *usageFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
		f.bodies = append(f.bodies, string(body))
		switch {
		case r.URL.Path == "/api/v1/usage/report" && r.URL.Query().Get("format") == "csv":
			w.Header().Set("Content-Type", "text/csv")
			_, _ = w.Write([]byte("resource_kind,resource\napp,web\n"))
		case r.URL.Path == "/api/v1/usage/report":
			_, _ = w.Write([]byte(`{"period":{"from":"2026-10-01T00:00:00Z","to":"2026-10-12T00:00:00Z"},"currency":"USD","mode":"node","configured":true,"includes_unallocated":true,
				"totals":{"total":12.5,"allocated":8,"unallocated":4.5,"previous":10,"delta_pct":25},"forecast":30,
				"projects":[{"key":"p1","total":8,"previous":6,"delta_pct":33.3,"daily":[]}],"labels":{"project:p1":"Storefront"}}`))
		case r.URL.Path == "/api/v1/usage/settings":
			_, _ = w.Write([]byte(`{"settings":{"mode":"unit","currency":"USD","basis":"avg","cpu_weight_pct":50},"nodes":[{"node_id":"local","name":"This server","monthly_price":40,"vcpus":4,"memory_gib":8}]}`))
		case r.URL.Path == "/api/v1/usage/budgets" && r.Method == http.MethodPut:
			_, _ = w.Write([]byte(`{"id":"bud_1","scope":"project","scope_id":"p1","monthly":100,"warn_pct":80}`))
		case r.URL.Path == "/api/v1/usage/budgets":
			_, _ = w.Write([]byte(`{"currency":"USD","budgets":[{"id":"bud_1","scope":"project","scope_id":"p1","name":"Storefront","monthly":100,"spent":85,"spent_pct":85,"forecast":120,"level":"warn"}]}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/usage/budgets/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

func TestRun_Usage(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantOut  string
		wantReqs []string
		wantBody string
	}{
		{"report table", []string{"usage", "report"}, "Storefront", []string{"GET /api/v1/usage/report?period=mtd"}, ""},
		{"report shows forecast and idle", []string{"usage", "report"}, "Forecast month end: 30.00 USD", []string{"GET /api/v1/usage/report?period=mtd"}, ""},
		{"report csv", []string{"usage", "report", "--csv", "--period", "30d"}, "app,web", []string{"GET /api/v1/usage/report?format=csv&period=30d"}, ""},
		{"settings show", []string{"usage", "settings"}, "mode: unit", []string{"GET /api/v1/usage/settings"}, ""},
		{"settings set", []string{"usage", "settings", "set", "--mode", "node", "--node", "local=40:4:8", "--reprice-month"}, "mode: unit",
			[]string{"GET /api/v1/usage/settings", "PUT /api/v1/usage/settings"}, `"monthly_price":40`},
		{"budgets list", []string{"usage", "budgets"}, "Storefront", []string{"GET /api/v1/usage/budgets"}, ""},
		{"budget alias", []string{"budget"}, "Storefront", []string{"GET /api/v1/usage/budgets"}, ""},
		{"budgets set", []string{"usage", "budgets", "set", "project", "p1", "--monthly", "100"}, "bud_1", []string{"PUT /api/v1/usage/budgets"}, `"monthly":100`},
		{"budgets delete", []string{"usage", "budgets", "delete", "bud_1"}, "deleted budget bud_1", []string{"DELETE /api/v1/usage/budgets/bud_1"}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &usageFake{}
			srv := httptest.NewServer(f.handler())
			defer srv.Close()
			out, _ := runCLIExpectOK(t, append(tc.args, "--api-url", srv.URL))
			if !strings.Contains(out, tc.wantOut) {
				t.Fatalf("output %q missing %q", out, tc.wantOut)
			}
			if strings.Join(f.requests, "|") != strings.Join(tc.wantReqs, "|") {
				t.Fatalf("requests = %v, want %v", f.requests, tc.wantReqs)
			}
			if tc.wantBody != "" && !strings.Contains(f.bodies[len(f.bodies)-1], tc.wantBody) {
				t.Fatalf("body %q missing %q", f.bodies[len(f.bodies)-1], tc.wantBody)
			}
		})
	}
}

func TestRun_UsageUsageErrors(t *testing.T) {
	look := func(string) (string, bool) { return "", false }
	var stdout, stderr strings.Builder
	if code := runUsage("cli", nil, &stdout, &stderr, look); code != exitUsage {
		t.Fatalf("no args = %d", code)
	}
	if code := runUsage("cli", []string{"bogus"}, &stdout, &stderr, look); code != exitUsage {
		t.Fatalf("unknown verb = %d", code)
	}
	if code := runUsageBudgets("cli", []string{"set", "project", "p1"}, &stdout, &stderr, look); code != exitUsage {
		t.Fatalf("set without --monthly = %d", code)
	}
}
