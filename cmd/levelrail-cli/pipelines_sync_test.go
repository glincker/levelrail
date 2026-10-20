package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func TestRun_PipelinesSyncAndTriggers(t *testing.T) {
	var truthBody map[string]bool
	var holdBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/pipeline-sync":
			_ = json.NewEncoder(w).Encode(apiclient.PipelineSyncResult{SHA: "abc123", Dir: ".pipelines", Items: []apiclient.PipelineSyncItem{
				{File: "ci.yaml", Name: "ci", Outcome: "diverged", Message: "edited since the last sync"},
			}})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/apps/web/pipeline-sync":
			_ = json.NewDecoder(r.Body).Decode(&truthBody)
			_ = json.NewEncoder(w).Encode(apiclient.PipelineSyncStatus{Connected: true, RepoIsTruth: truthBody["repo_is_truth"]})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/pipeline-triggers":
			_ = json.NewEncoder(w).Encode([]apiclient.PipelineTrigger{{Event: "pull_request", Pipeline: "ci", Decision: "skipped", Reason: "fork blocked", CreatedAt: time.Now()}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/apps/web/pipeline-runs/r9":
			_ = json.NewEncoder(w).Encode(apiclient.PipelineRunResource{ID: "r9", Status: "queued", Hold: &apiclient.PipelineHold{State: "pending", Reason: "fork"}})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/apps/web/pipeline-runs/r9/hold":
			_ = json.NewDecoder(r.Body).Decode(&holdBody)
			_ = json.NewEncoder(w).Encode(map[string]string{"decision": holdBody["decision"]})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	call := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := run("cli", append(args, "--api-url", srv.URL), &stdout, &stderr, envMap())
		return code, stdout.String(), stderr.String()
	}

	code, out, errOut := call("pipelines", "sync", "web")
	if code != exitOK || !strings.Contains(out, "synced from abc123") || !strings.Contains(out, "diverged") {
		t.Fatalf("sync: exit %d out %q err %q", code, out, errOut)
	}
	code, out, errOut = call("pipelines", "sync", "web", "--repo-truth", "true")
	if code != exitOK || !truthBody["repo_is_truth"] || !strings.Contains(out, "true") {
		t.Fatalf("repo-truth: exit %d body %v out %q err %q", code, truthBody, out, errOut)
	}
	if code, _, errOut = call("pipelines", "sync", "web", "--repo-truth", "maybe"); code != exitValidation {
		t.Fatalf("bad --repo-truth: exit %d err %q", code, errOut)
	}
	code, out, errOut = call("pipelines", "triggers", "web")
	if code != exitOK || !strings.Contains(out, "fork blocked") || !strings.Contains(out, "skipped") {
		t.Fatalf("triggers: exit %d out %q err %q", code, out, errOut)
	}
	code, out, errOut = call("pipelines", "approve", "web", "r9", "--reject")
	if code != exitOK || holdBody["decision"] != "rejected" || !strings.Contains(out, "held for approval") {
		t.Fatalf("hold: exit %d body %v out %q err %q", code, holdBody, out, errOut)
	}
}
