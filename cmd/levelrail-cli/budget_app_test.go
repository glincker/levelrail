package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsAppBudgetSet(t *testing.T) {
	tests := []struct {
		args []string
		want bool
	}{
		{[]string{"web", "--egress", "1GiB"}, true},
		{[]string{"--egress", "1GiB", "web"}, true},
		{[]string{"project", "p1", "--monthly", "100"}, false},
		{[]string{"team", "t1", "--monthly=5"}, false},
		{[]string{"p1", "--monthly", "100"}, false},
	}
	for _, tc := range tests {
		if got := isAppBudgetSet(tc.args); got != tc.want {
			t.Errorf("isAppBudgetSet(%v) = %v", tc.args, got)
		}
	}
}

type budgetFake struct {
	requests []string
	bodies   []string
	exists   bool
}

func (f *budgetFake) handler() http.Handler {
	const status = `{"app":"web","monthly_egress_bytes":536870912000,"monthly_cost_ceiling":0,"thresholds":[50,80,100],"action":"throttle","throttle_rps":5,
		"period":"2026-10","egress_bytes":107374182400,"cost":0,"egress_pct":20,"cost_pct":0,"pct":20,"enforced":"none","resets_at":"2026-11-01T00:00:00Z",
		"condition":{"healthy":true,"reason":"WithinBudget","message":"Usage is at 20% of the limit."},"cost_priced":false}`
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.requests = append(f.requests, r.Method+" "+r.URL.Path)
		f.bodies = append(f.bodies, string(body))
		switch {
		case r.URL.Path == "/api/v1/apps/web/budget" && r.Method == http.MethodGet && !f.exists:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no budget is set for this app"}`))
		case r.URL.Path == "/api/v1/apps/web/budget" && r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/v1/apps/web/budget/history":
			_, _ = w.Write([]byte(`{"events":[{"id":1,"app":"web","period":"2026-10","kind":"threshold","detail":"reached 50% of the limit","pct":50,"at":"2026-10-10T12:00:00Z"}]}`))
		case r.URL.Path == "/api/v1/apps/web/budget":
			_, _ = w.Write([]byte(status))
		default:
			http.NotFound(w, r)
		}
	})
}

func TestRun_AppBudget(t *testing.T) {
	tests := []struct {
		name     string
		exists   bool
		args     []string
		wantOut  string
		wantReqs string
		wantBody string
	}{
		{"show missing", false, []string{"budget", "show", "web"}, "has no budget", "GET /api/v1/apps/web/budget", ""},
		{"show", true, []string{"budget", "show", "web"}, "100.0 GiB of 500.0 GiB (20%)", "GET /api/v1/apps/web/budget", ""},
		{"show history", true, []string{"budget", "show", "web", "--history"}, "reached 50% of the limit", "GET /api/v1/apps/web/budget|GET /api/v1/apps/web/budget/history", ""},
		{"set new", false, []string{"budget", "set", "web", "--egress", "500GiB", "--action", "throttle"}, "WithinBudget", "GET /api/v1/apps/web/budget|PUT /api/v1/apps/web/budget", `"monthly_egress_bytes":536870912000`},
		{"set merges existing", true, []string{"budget", "set", "web", "--action", "pause"}, "WithinBudget", "GET /api/v1/apps/web/budget|PUT /api/v1/apps/web/budget", `"monthly_egress_bytes":536870912000`},
		{"set thresholds", true, []string{"budget", "set", "web", "--thresholds", "25,75"}, "WithinBudget", "GET /api/v1/apps/web/budget|PUT /api/v1/apps/web/budget", `"thresholds":[25,75]`},
		{"clear", true, []string{"budget", "clear", "web"}, "removed the budget of web", "DELETE /api/v1/apps/web/budget", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := &budgetFake{exists: tc.exists}
			srv := httptest.NewServer(f.handler())
			defer srv.Close()
			out, _ := runCLIExpectOK(t, append(tc.args, "--api-url", srv.URL))
			if !strings.Contains(out, tc.wantOut) {
				t.Errorf("output %q missing %q", out, tc.wantOut)
			}
			if got := strings.Join(f.requests, "|"); got != tc.wantReqs {
				t.Errorf("requests = %s, want %s", got, tc.wantReqs)
			}
			if tc.wantBody != "" && !strings.Contains(f.bodies[len(f.bodies)-1], tc.wantBody) {
				t.Errorf("body %q missing %q", f.bodies[len(f.bodies)-1], tc.wantBody)
			}
		})
	}
}

func TestRun_AppBudgetUsageErrors(t *testing.T) {
	var stdout, stderr strings.Builder
	for _, args := range [][]string{{"show"}, {"clear"}, {"show", "a", "b"}} {
		if code := runBudget("cli", args, &stdout, &stderr, envMap()); code != exitUsage {
			t.Errorf("%v = %d, want usage error", args, code)
		}
	}
	if code := runBudget("cli", []string{"set", "web", "--egress", "lots"}, &stdout, &stderr, envMap()); code == exitOK {
		t.Error("bad size accepted")
	}
}
