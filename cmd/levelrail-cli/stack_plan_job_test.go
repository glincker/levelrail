package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
)

func fastPlanPoll(t *testing.T) {
	t.Helper()
	oldMin, oldMax, oldSleep := planPollFirst, planPollCeil, planPollSleep
	planPollFirst, planPollCeil = time.Millisecond, 2*time.Millisecond
	planPollSleep = func(ctx context.Context, d time.Duration) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
			return nil
		}
	}
	t.Cleanup(func() { planPollFirst, planPollCeil, planPollSleep = oldMin, oldMax, oldSleep })
}

// jobServer answers 202 on plan, then the given statuses on polls (the last one repeats).
func jobServer(t *testing.T, statuses []string, code, msg string, plan iac.Plan) (*httptest.Server, *[]string, *atomic.Int32) {
	t.Helper()
	var calls []string
	var polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/plan"):
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "job1", "stack": "shop", "status": "queued", "poll_url": "/api/v1/stacks/shop/plan-jobs/job1"})
		case strings.HasSuffix(r.URL.Path, "/plan-jobs/job1"):
			i := int(polls.Add(1)) - 1
			if i >= len(statuses) {
				i = len(statuses) - 1
			}
			body := map[string]any{"id": "job1", "stack": "shop", "status": statuses[i]}
			if statuses[i] == "done" {
				body["result"] = map[string]any{"stack": "shop", "stack_exists": true, "plan": plan}
			}
			if statuses[i] == "failed" {
				body["code"], body["error"] = code, msg
			}
			_ = json.NewEncoder(w).Encode(body)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &calls, &polls
}

func TestStackPlanJob_PollsUntilDone(t *testing.T) {
	fastPlanPoll(t)
	srv, calls, polls := jobServer(t, []string{"queued", "running", "done"}, "", "", pendingPlan())
	dir := stackTestDir(t)
	code, out, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--exit-code", "--supersede", "--api-url", srv.URL}, nil)
	if code != iacExitPending || polls.Load() != 3 || !strings.Contains(out, "Plan hash: abc123") {
		t.Fatalf("exit %d polls %d\n%s", code, polls.Load(), out)
	}
	if !strings.Contains((*calls)[0], "plan?supersede=true") {
		t.Errorf("supersede not sent: %v", *calls)
	}
	if code, _, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--api-url", srv.URL}, nil); code != exitOK {
		t.Fatalf("no --exit-code plan = %d", code)
	}
	_, jsonOut, _ := runIaC([]string{"stack", "plan", "shop", "-f", dir, "--json", "--api-url", srv.URL}, nil)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil || parsed["stack"] != "shop" || parsed["plan"] == nil {
		t.Errorf("--json is not the plan: %v\n%s", err, jsonOut)
	}
}

func TestStackPlanJob_FailedJobExitsOneWithCode(t *testing.T) {
	fastPlanPoll(t)
	srv, _, _ := jobServer(t, []string{"running", "failed"}, "plan_timeout", "the plan took too long", iac.Plan{})
	code, _, errOut := runIaC([]string{"stack", "plan", "shop", "-f", stackTestDir(t), "--api-url", srv.URL}, nil)
	if code != iacExitError || !strings.Contains(errOut, "plan_timeout") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func TestStackPlanJob_WaitTimeoutLeavesJobRunning(t *testing.T) {
	fastPlanPoll(t)
	srv, _, _ := jobServer(t, []string{"running"}, "", "", iac.Plan{})
	code, _, errOut := runIaC([]string{"stack", "plan", "shop", "-f", stackTestDir(t), "--wait-timeout", "30ms", "--api-url", srv.URL}, nil)
	if code != iacExitError || !strings.Contains(errOut, "keeps running") || !strings.Contains(errOut, "plan-jobs/job1") {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
}

func TestStackApprove_ConfirmDestroyIsCheckedLocally(t *testing.T) {
	f := &stackFake{t: t, plan: pendingPlan(), status: "applied"}
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	code, _, errOut := runIaC([]string{"stack", "approve", "shop", "4", "--confirm-destroy", "wrong", "--api-url", srv.URL}, nil)
	if code == exitOK || !strings.Contains(errOut, "--confirm-destroy") || len(f.calls) != 0 {
		t.Fatalf("exit %d calls %v stderr %q", code, f.calls, errOut)
	}
	runIaC([]string{"stack", "approve", "shop", "4", "--confirm-destroy", "shop", "--api-url", srv.URL}, nil)
	if len(f.rawBodies) != 1 || !strings.Contains(f.rawBodies[0], `"confirm_destroy":"shop"`) {
		t.Fatalf("body not sent: %v", f.rawBodies)
	}
}

func TestStackCloudState_ListAndRecover(t *testing.T) {
	var posted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cloud-state/quarantine"):
			_, _ = w.Write([]byte(`{"stack":"shop","quarantine":[{"id":3,"account":"oci","reason":"lease lost","fence":7,"created_at":"2026-11-13T10:00:00Z"}]}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/quarantine/3/resolve"):
			b := make([]byte, 200)
			n, _ := r.Body.Read(b)
			posted = string(b[:n])
			_, _ = w.Write([]byte(`{"stack":"shop","id":3,"resolved":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	code, out, _ := runIaC([]string{"stack", "cloud-state", "list", "shop", "--api-url", srv.URL}, nil)
	if code != exitOK || !strings.Contains(out, "lease lost") || !strings.Contains(out, "oci") {
		t.Fatalf("list %d\n%s", code, out)
	}
	if code, _, _ := runIaC([]string{"stack", "cloud-state", "recover", "shop", "3", "--confirm", "nope", "--api-url", srv.URL}, nil); code == exitOK || posted != "" {
		t.Fatalf("wrong confirm went through: %d %q", code, posted)
	}
	if code, _, _ := runIaC([]string{"stack", "cloud-state", "recover", "shop", "3", "--confirm", "shop", "--api-url", srv.URL}, nil); code != exitOK || !strings.Contains(posted, `"confirm":"shop"`) {
		t.Fatalf("recover %d %q", code, posted)
	}
}
