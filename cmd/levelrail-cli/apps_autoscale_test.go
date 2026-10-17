package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/clitree"
)

const autoscaleStatusJSON = `{"app":"web","min_replicas":2,"max_replicas":6,"metric":"cpu_percent","target":60,
"scale_up_tolerance_pct":10,"scale_down_tolerance_pct":30,"scale_up_cooldown_secs":60,"scale_down_cooldown_secs":600,
"stabilization_secs":300,"max_step_up":4,"max_step_down":1,"effective_max":6,"current":3,"desired":4,"value":82.5,"value_known":true,
"blocked_by":"","flaps_1h":0,"flapping":false,"condition":{"healthy":true,"reason":"ScaledUp","message":"adding 1"}}`

func autoscaleServer(t *testing.T, put *map[string]any, missing bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPut:
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, put)
			_, _ = w.Write([]byte(autoscaleStatusJSON))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/history"):
			_, _ = w.Write([]byte(`{"events":[{"id":1,"app":"web","kind":"scaled_up","current":3,"desired":4,"value":82.5,"reason":"ScaledUp","blocked_by":"","detail":"adding 1","at":"2026-10-17T12:00:00Z"}]}`))
		case missing:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"no autoscale policy is set for this app"}`))
		default:
			_, _ = w.Write([]byte(autoscaleStatusJSON))
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestRun_AppsAutoscale_Show(t *testing.T) {
	var put map[string]any
	srv := autoscaleServer(t, &put, false)
	out, _ := runCLIExpectOK(t, []string{"apps", "autoscale", "show", "web", "--api-url", srv.URL})
	for _, want := range []string{"2 to 6 replicas", "3 now, 4 desired", "82.5", "ScaledUp"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestRun_AppsAutoscale_ShowWithoutPolicySuggestsNextStep(t *testing.T) {
	var put map[string]any
	srv := autoscaleServer(t, &put, true)
	out, _ := runCLIExpectOK(t, []string{"apps", "autoscale", "show", "web", "--api-url", srv.URL})
	if !strings.Contains(out, "no autoscale policy") || !strings.Contains(out, "apps autoscale set web") {
		t.Errorf("output = %q", out)
	}
}

func TestRun_AppsAutoscale_SetNewAndMerge(t *testing.T) {
	var put map[string]any
	srv := autoscaleServer(t, &put, true)
	runCLIExpectOK(t, []string{"apps", "autoscale", "set", "web", "--max", "5", "--metric", "requests_per_replica", "--target", "100", "--scale-down-cooldown", "20m", "--api-url", srv.URL})
	if put["max_replicas"] != float64(5) || put["min_replicas"] != float64(1) || put["metric"] != "requests_per_replica" || put["scale_down_cooldown_secs"] != float64(1200) {
		t.Errorf("PUT body = %v", put)
	}

	put = nil
	srv = autoscaleServer(t, &put, false)
	runCLIExpectOK(t, []string{"apps", "autoscale", "set", "web", "--target", "70", "--api-url", srv.URL})
	if put["target"] != float64(70) || put["max_replicas"] != float64(6) || put["metric"] != "cpu_percent" || put["stabilization_secs"] != float64(300) {
		t.Errorf("a flag must change only its own field, PUT body = %v", put)
	}
}

func TestRun_AppsAutoscale_SetNewNeedsTheRequiredFlags(t *testing.T) {
	var put map[string]any
	srv := autoscaleServer(t, &put, true)
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "autoscale", "set", "web", "--max", "5", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if got != exitUsage || !strings.Contains(stderr.String(), "--metric") {
		t.Errorf("exit = %d stderr = %q", got, stderr.String())
	}
}

func TestRun_AppsAutoscale_ClearAndHistory(t *testing.T) {
	var put map[string]any
	srv := autoscaleServer(t, &put, false)
	out, _ := runCLIExpectOK(t, []string{"apps", "autoscale", "clear", "web", "--api-url", srv.URL})
	if !strings.Contains(out, "stay where they are") {
		t.Errorf("clear output = %q", out)
	}
	out, _ = runCLIExpectOK(t, []string{"apps", "autoscale", "history", "web", "--api-url", srv.URL})
	if !strings.Contains(out, "scaled_up") || !strings.Contains(out, "3 to 4") {
		t.Errorf("history output = %q", out)
	}
}

func TestRun_AppsAutoscale_UnknownVerb(t *testing.T) {
	var stdout, stderr strings.Builder
	if got := run("levelrail-cli-test", []string{"apps", "autoscale", "fly"}, &stdout, &stderr, envMap()); got != exitUsage {
		t.Errorf("exit = %d", got)
	}
}

func TestAutoscaleIsInTheCompletionTree(t *testing.T) {
	node := clitree.Tree["apps"].Subs["autoscale"]
	if node == nil {
		t.Fatal("apps autoscale is missing from the command tree")
	}
	for _, verb := range []string{"show", "set", "clear", "history"} {
		if _, ok := node.Subs[verb]; !ok {
			t.Errorf("apps autoscale %s is missing from the command tree", verb)
		}
	}
}
