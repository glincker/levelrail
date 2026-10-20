package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func gateServer(t *testing.T, pages []apiclient.DeployGateResource, paths *[]string) *httptest.Server {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*paths = append(*paths, r.URL.Path)
		i := min(int(calls.Add(1))-1, len(pages)-1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pages[i])
	}))
	t.Cleanup(srv.Close)
	return srv
}

func gateEvent(phase, status, msg string) apiclient.GateEventResource {
	return apiclient.GateEventResource{Phase: phase, Status: status, Message: msg, At: time.Now()}
}

func TestRun_DeployStatusWatch(t *testing.T) {
	first := apiclient.DeployGateResource{DeployID: "dep_9", Image: "web:9", Stage: "gating",
		Events: []apiclient.GateEventResource{gateEvent("readiness", "passed", "first readiness check passed")}}
	second := first
	second.Stage, second.Terminal, second.Failed, second.Reason = "rolled_back", true, true, "PostDeployErrorRate"
	second.Events = append(append([]apiclient.GateEventResource{}, first.Events...),
		gateEvent("cutover", "passed", "traffic moved to 1 ready replica(s)"),
		gateEvent("rollback", "passed", "rolled back to deploy dep_8"))
	var paths []string
	srv := gateServer(t, []apiclient.DeployGateResource{first, second}, &paths)

	var stdout, stderr strings.Builder
	code := run("levelrail-cli-test", []string{"deploy", "status", "web", "--watch", "--interval", "10ms", "--api-url", srv.URL}, &stdout, &stderr, envMap())
	if code != exitDeployNotHealthy {
		t.Fatalf("exit = %d, want %d (rolled back); stderr %s", code, exitDeployNotHealthy, stderr.String())
	}
	if paths[0] != "/api/v1/apps/web/deploys/latest/gate" || paths[1] != "/api/v1/apps/web/deploys/dep_9/gate" {
		t.Errorf("paths = %v, want latest then the resolved id", paths)
	}
	out := stdout.String()
	if strings.Count(out, "first readiness check passed") != 1 || !strings.Contains(out, "rolled back to deploy dep_8") {
		t.Errorf("stdout = %s, want each step printed once", out)
	}
}

func TestRun_AppsDeploysGateExitCodes(t *testing.T) {
	tests := []struct {
		name string
		gate apiclient.DeployGateResource
		want int
	}{
		{"cleared", apiclient.DeployGateResource{DeployID: "d", Stage: "cleared", Terminal: true}, exitOK},
		{"still watching", apiclient.DeployGateResource{DeployID: "d", Stage: "watching"}, exitOK},
		{"gate failed", apiclient.DeployGateResource{DeployID: "d", Stage: "gate_failed", Terminal: true, Failed: true}, exitDeployNotHealthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var paths []string
			srv := gateServer(t, []apiclient.DeployGateResource{tt.gate}, &paths)
			code := run("levelrail-cli-test", []string{"apps", "deploys", "gate", "web", "d", "--api-url", srv.URL}, io.Discard, io.Discard, envMap())
			if code != tt.want {
				t.Errorf("exit = %d, want %d", code, tt.want)
			}
		})
	}
}

func TestRun_AppsDeploySafetySet(t *testing.T) {
	five := 5
	var put apiclient.DeploySafetyOverrides
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		res := apiclient.DeploySafetyResource{Overrides: apiclient.DeploySafetyOverrides{SuccessThreshold: &five}}
		if r.Method == http.MethodPut {
			if err := json.NewDecoder(r.Body).Decode(&put); err != nil {
				t.Errorf("decode PUT: %v", err)
			}
			res.Overrides = put
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
	}))
	defer srv.Close()

	runCLIExpectOK(t, []string{"apps", "deploy-safety", "set", "web", "--stable-window", "20s", "--success-threshold", "default", "--auto-rollback", "false", "--api-url", srv.URL})
	if put.SuccessThreshold != nil || put.StableWindow == nil || *put.StableWindow != "20s" || put.AutoRollback == nil || *put.AutoRollback {
		t.Errorf("PUT overrides = %+v, want threshold cleared, 20s window, auto-rollback off", put)
	}

	var stderr strings.Builder
	code := run("levelrail-cli-test", []string{"apps", "deploy-safety", "set", "web", "--stable-window", "soon", "--api-url", srv.URL}, io.Discard, &stderr, envMap())
	if code != exitValidation {
		t.Errorf("bad duration exit = %d, want %d (%s)", code, exitValidation, stderr.String())
	}
}
