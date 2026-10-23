package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const timelineJSON = `{"deploy_id":"dep_1","app":"web","verdict":"succeeded","events":[
{"id":"trigger","at":"2026-10-01T12:00:00Z","kind":"trigger","phase":"trigger","status":"info","code":"trigger.git_push","title":"Git push received","detail":"main abc1234 | by ana"},
{"id":"log:0","at":"2026-10-01T12:00:01Z","kind":"log","phase":"build","status":"info","code":"log.line","title":"Build output","detail":"step 1","stream":"stdout"},
{"id":"watch","at":"2026-10-01T12:00:40Z","kind":"watch","phase":"watch","status":"failed","code":"watch.crashloop","title":"Crashloop after deploy","reason":"PostDeployCrashloop"},
{"id":"traffic:impact","at":"2026-10-01T12:05:00Z","kind":"traffic_impact","phase":"watch","status":"info","code":"traffic.impact","title":"Traffic impact around cutover","impact":{"complete":true,"before":{"has_traffic":true,"rate_per_sec":10,"error_rate_5xx":0.01,"p95_ms":80},"after":{"has_traffic":true,"rate_per_sec":9,"error_rate_5xx":0.2,"p95_ms":400}}}
]}`

func TestRun_DeploymentsTimeline_Text(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(timelineJSON))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"deployments", "timeline", "dep_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/deployments/dep_1/timeline" {
		t.Fatalf("path = %q", gotPath)
	}
	for _, want := range []string{"trigger.git_push", "watch.crashloop", "[PostDeployCrashloop]", "5xx", "1 build log lines hidden"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "step 1") {
		t.Errorf("log lines must be hidden without --logs:\n%s", stdout)
	}
	withLogs, _ := runCLIExpectOK(t, []string{"deployments", "timeline", "dep_1", "--logs", "--api-url", srv.URL})
	if !strings.Contains(withLogs, "step 1") {
		t.Errorf("--logs should show build output:\n%s", withLogs)
	}
}

func followServer(t *testing.T, verdict string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/deployments/dep_1/timeline/stream" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, ": connected\n\n")
		_, _ = fmt.Fprint(w, `data: {"type":"snapshot","verdict":"running","live":true,"timeline":{"events":[{"id":"trigger","at":"2026-10-01T12:00:00Z","kind":"trigger","status":"info","code":"trigger.manual","title":"Deploy started manually"}]}}`+"\n\n")
		_, _ = fmt.Fprint(w, `data: {"type":"event","live":true,"event":{"id":"outcome","at":"2026-10-01T12:00:30Z","kind":"outcome","status":"ok","code":"outcome.x","title":"Deploy finished"}}`+"\n\n")
		_, _ = fmt.Fprintf(w, `data: {"type":"end","verdict":%q}`+"\n\n", verdict)
	}))
}

func TestRun_DeploymentsTimeline_FollowSucceeds(t *testing.T) {
	srv := followServer(t, "succeeded")
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"deployments", "timeline", "dep_1", "--follow", "--api-url", srv.URL})
	if !strings.Contains(stdout, "trigger.manual") || !strings.Contains(stdout, "outcome.x") || !strings.Contains(stdout, "verdict: succeeded") {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestRun_DeploymentsTimeline_FollowFailedExitsNonZero(t *testing.T) {
	srv := followServer(t, "rolled_back")
	defer srv.Close()
	runCLIExpectAPIError(t, []string{"deployments", "timeline", "dep_1", "--follow", "--api-url", srv.URL})
}

func TestRun_DeploymentsTimeline_Compare(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/deployments/dep_1/timeline/compare" || r.URL.Query().Get("with") != "dep_2" {
			t.Errorf("url = %s", r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"a":{"deploy_id":"dep_1","verdict":"succeeded"},"b":{"deploy_id":"dep_2","verdict":"rolled_back"},
"phases":[{"phase":"build","a_ms":1000,"b_ms":3000,"delta_ms":2000}],"only_a":[],"only_b":["watch.crashloop"]}`))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"deployments", "timeline", "dep_1", "--compare", "dep_2", "--api-url", srv.URL})
	for _, want := range []string{"build", "+2000ms", "only in B: watch.crashloop"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_DeploymentsTimeline_NeedsID(t *testing.T) {
	runCLIExpectUsageError := func() int {
		var o, e strings.Builder
		return run("cli-test", []string{"deployments", "timeline"}, &o, &e, envMap())
	}
	if got := runCLIExpectUsageError(); got != exitUsage {
		t.Fatalf("exit = %d, want usage", got)
	}
}
