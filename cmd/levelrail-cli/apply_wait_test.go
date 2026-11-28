package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/iac"
)

// waitFake serves the apply endpoints plus a scripted app list: each poll
// answers the next entry, the last one repeats.
type waitFake struct {
	apply   http.Handler
	polls   [][]map[string]any
	pollIdx int
}

func (f *waitFake) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps" {
			i := min(f.pollIdx, len(f.polls)-1)
			f.pollIdx++
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(f.polls[i])
			return
		}
		f.apply.ServeHTTP(w, r)
	})
}

func appRow(label, variant, reason, msg string) map[string]any {
	return map[string]any{"name": "web", "status": map[string]any{"label": label, "variant": variant, "reason": reason, "message": msg}}
}

func appliedStackHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		plan := pendingPlan()
		switch {
		case strings.HasSuffix(r.URL.Path, "/apply"):
			res := iac.ApplyResult{Plan: plan, Applied: 1, Results: []iac.ItemResult{{Change: plan.Changes[0], Status: iac.StatusApplied}}}
			rev := map[string]any{"number": 4, "status": "applied", "plan_hash": "h1", "actor": "ci", "created_at": "2026-10-25T10:00:00Z"}
			_ = json.NewEncoder(w).Encode(map[string]any{"stack": "shop", "revision": rev, "result": res})
		default:
			http.NotFound(w, r)
		}
	})
}

func TestApplyWait_FailedAppSetsExitAndShowsReason(t *testing.T) {
	f := &iacFake{plan: pendingPlan()}
	wf := &waitFake{apply: f.handler(t), polls: [][]map[string]any{
		{appRow("Reconciling", "muted", "", "")},
		{appRow("Attention needed", "destructive", "ImagePullFailed", "pull access denied for a:2")},
	}}
	srv := httptest.NewServer(wf.handler())
	defer srv.Close()

	code, out, _ := runIaC([]string{"apply", "-f", writeInfra(t), "--yes", "--wait", "--ready-interval", "1ms", "--ready-timeout", "5s", "--api-url", srv.URL}, nil)
	if code != iacExitError {
		t.Fatalf("exit = %d, want %d\n%s", code, iacExitError, out)
	}
	for _, want := range []string{"App status:", "failed", "web", "ImagePullFailed", "pull access denied for a:2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestApplyWait_RunningAppExitsZero(t *testing.T) {
	f := &iacFake{plan: pendingPlan()}
	wf := &waitFake{apply: f.handler(t), polls: [][]map[string]any{{appRow("Healthy", "success", "", "")}}}
	srv := httptest.NewServer(wf.handler())
	defer srv.Close()

	code, out, _ := runIaC([]string{"apply", "-f", writeInfra(t), "--yes", "--wait", "--ready-interval", "1ms", "--api-url", srv.URL}, nil)
	if code != exitOK || !strings.Contains(out, "running  web") {
		t.Fatalf("exit = %d\n%s", code, out)
	}
}

func TestApplyWait_TimeoutIsFailureAndJSON(t *testing.T) {
	f := &iacFake{plan: pendingPlan()}
	wf := &waitFake{apply: f.handler(t), polls: [][]map[string]any{{appRow("Reconciling", "muted", "", "")}}}
	srv := httptest.NewServer(wf.handler())
	defer srv.Close()

	code, out, _ := runIaC([]string{"apply", "-f", writeInfra(t), "--yes", "--wait", "--json", "--ready-interval", "1ms", "--ready-timeout", "30ms", "--api-url", srv.URL}, nil)
	if code != iacExitError {
		t.Fatalf("exit = %d, want %d\n%s", code, iacExitError, out)
	}
	var got struct {
		Wait []appWaitResult `json:"wait"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, out)
	}
	if len(got.Wait) != 1 || got.Wait[0].State != waitStateTimeout || got.Wait[0].Label != "Reconciling" {
		t.Errorf("wait = %+v", got.Wait)
	}
}

func TestApplyWait_EnvDefaultsAndNoWaitUnchanged(t *testing.T) {
	f := &iacFake{plan: pendingPlan()}
	wf := &waitFake{apply: f.handler(t), polls: [][]map[string]any{{appRow("Healthy", "success", "", "")}}}
	srv := httptest.NewServer(wf.handler())
	defer srv.Close()

	env := map[string]string{applyWaitIntervalEnv: "1ms", applyWaitTimeoutEnv: "5s"}
	code, out, errOut := runIaC([]string{"apply", "-f", writeInfra(t), "--yes", "--wait", "--api-url", srv.URL}, env)
	if code != exitOK || !strings.Contains(errOut, "waiting up to 5s") {
		t.Fatalf("exit = %d, stderr %q\n%s", code, errOut, out)
	}
	wf.pollIdx = 0
	code, out, _ = runIaC([]string{"apply", "-f", writeInfra(t), "--yes", "--api-url", srv.URL}, nil)
	if code != exitOK || strings.Contains(out, "App status:") || wf.pollIdx != 0 {
		t.Errorf("without --wait the app list must not be polled: exit %d polls %d\n%s", code, wf.pollIdx, out)
	}
}

func TestStackApplyWait_FailedAppSetsExit(t *testing.T) {
	wf := &waitFake{apply: appliedStackHandler(), polls: [][]map[string]any{
		{appRow("Attention needed", "destructive", "CrashLoopBackOff", "container exited 1")},
	}}
	srv := httptest.NewServer(wf.handler())
	defer srv.Close()

	code, out, _ := runIaC([]string{"stack", "apply", "shop", "-f", stackTestDir(t), "--yes", "--wait", "--ready-interval", "1ms", "--api-url", srv.URL}, nil)
	if code != iacExitError {
		t.Fatalf("exit = %d, want %d\n%s", code, iacExitError, out)
	}
	if !strings.Contains(out, "CrashLoopBackOff") || !strings.Contains(out, "container exited 1") {
		t.Errorf("output lacks the failure reason:\n%s", out)
	}
}

func TestApplyWait_RejectedWithServerSide(t *testing.T) {
	code, _, errOut := runIaC([]string{"apply", "-f", "x", "--wait", "--server-side", "--api-url", "http://127.0.0.1:1"}, nil)
	if code != iacExitError || !strings.Contains(errOut, "--wait is not supported with --server-side") {
		t.Errorf("exit %d stderr %q", code, errOut)
	}
}
