package failure

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestClassifyFixtures(t *testing.T) {
	at := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name      string
		in        Input
		wantCode  string
		retryable bool
	}{
		{"dockerfile missing", Input{Status: "failed", Error: "deploy: service \"web\": build: failed to read dockerfile: open Dockerfile: no such file or directory"}, CodeDockerfileError, false},
		{"dockerfile syntax", Input{Status: "failed", Error: "deploy: service \"web\": build: dockerfile parse error on line 3: unknown instruction: RUNN"}, CodeDockerfileError, false},
		{"dependency install", Input{Status: "failed", Error: "deploy: service \"web\": build: exit 1", LogLines: []string{"npm ERR! code ERESOLVE", "npm ERR! could not resolve"}}, CodeDependencyInstall, false},
		{"build oom", Input{Status: "failed", FailingStep: "building", Error: "deploy: service \"web\": build: process \"/bin/sh -c npm run build\" did not complete successfully: signal: killed"}, CodeBuildOOM, false},
		{"build timeout", Input{Status: "failed", Error: "deploy: service \"web\": build: context deadline exceeded"}, CodeBuildTimeout, true},
		{"image pull denied", Input{Status: "failed", Error: "pull access denied for acme/web, repository does not exist or may require authorization"}, CodeImagePullFailed, false},
		{"image not found", Input{Status: "failed", Error: "manifest unknown: manifest unknown"}, CodeImagePullFailed, false},
		{"port not listening", Input{Condition: &Condition{Reason: "ReadinessFailed", Message: "GET http://172.18.0.3:3000/healthz: dial tcp 172.18.0.3:3000: connect: connection refused"}}, CodePortNotListening, false},
		{"health check failed", Input{Condition: &Condition{Reason: "ReadinessFailed", Message: "readiness probe returned status 503"}}, CodeHealthCheckFailed, false},
		{"container exit code", Input{Condition: &Condition{Reason: "ExitedDuringReadiness", Message: "container exited with code 2 during readiness"}}, CodeContainerCrashed, false},
		{"crashloop", Input{Status: "failed", Error: "unhealthy", Crashloop: true}, CodeContainerCrashed, false},
		{"runtime oom", Input{Condition: &Condition{Reason: "OOMKilledDuringReadiness", Message: "container was OOM killed"}}, CodeOOMKilled, false},
		{"missing env", Input{Status: "failed", Error: "deploy: service \"web\": env var \"API_KEY\" is required but no secret value has been set for it yet"}, CodeMissingEnv, false},
		{"registry push", Input{Status: "failed", FailingStep: "pushing", Error: "deploy: service \"web\": failed to push registry.local/web:abc: 502 Bad Gateway"}, CodeRegistryPushFailed, true},
		{"disk full", Input{Status: "failed", Error: "write /var/lib/docker/tmp: no space left on device"}, CodeDiskFull, false},
		{"freeze hold", Input{Status: "held", Reason: "Frozen", Error: "Frozen: release interrupted by restart"}, CodeFreezeWindow, true},
		{"approval hold", Input{Status: "held", Reason: "AwaitingApproval"}, CodeApprovalPending, true},
		{"scan gate", Input{Status: "failed", Error: "supply chain gate blocked this release: 2 critical vulnerabilities"}, CodeScanGateBlocked, false},
		{"rollback gc", Input{Status: "failed", Error: "image web:1 was garbage collected and can no longer be rolled back to"}, CodeRollbackTargetGone, false},
		{"docker unreachable", Input{Status: "failed", Error: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock"}, CodeDockerUnreachable, true},
		{"unknown", Input{Status: "failed", Error: "something entirely unexpected happened"}, CodeUnknown, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.App, tc.in.DeployID, tc.in.At = "web", "da_1", at
			got, ok := Classify(tc.in, Options{})
			if !ok {
				t.Fatalf("Classify reported not failing")
			}
			if got.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q (cause %q)", got.Code, tc.wantCode, got.Cause)
			}
			if got.Retryable != tc.retryable {
				t.Errorf("retryable = %v, want %v", got.Retryable, tc.retryable)
			}
			if got.SuggestedFix == "" || got.Cause == "" || got.DocsURL != "/deploy-failures#"+tc.wantCode {
				t.Errorf("incomplete object: %+v", got)
			}
			if got.App != "web" || got.DeployID != "da_1" || !got.At.Equal(at) {
				t.Errorf("identity fields not carried: %+v", got)
			}
		})
	}
}

func TestClassifyNotFailing(t *testing.T) {
	for _, status := range []string{"succeeded", "running", "queued", "canceled", "superseded"} {
		if _, ok := Classify(Input{Status: status}, Options{}); ok {
			t.Errorf("status %q classified as failing", status)
		}
	}
	if _, ok := Classify(Input{Status: "succeeded", Condition: &Condition{Reason: "Deployed"}}, Options{}); ok {
		t.Error("healthy rollout classified as failing")
	}
}

func TestUnknownKeepsExcerpt(t *testing.T) {
	got, _ := Classify(Input{Status: "failed", Error: "weird failure", LogLines: []string{"line a", "line b"}}, Options{})
	if got.Code != CodeUnknown || !strings.Contains(got.LogExcerpt, "weird failure") || !strings.Contains(got.LogExcerpt, "line b") {
		t.Errorf("unknown failure lost its excerpt: %+v", got)
	}
}

func TestExcerptCappedAndRedacted(t *testing.T) {
	var logs []string
	for i := 0; i < 100; i++ {
		logs = append(logs, fmt.Sprintf("step %d output", i))
	}
	logs = append(logs, "npm ERR! code ERESOLVE", "curl https://user:hunter2@registry.example.com/x", "Authorization: Bearer abcdefghijklmnop12345", "trailing 1", "trailing 2", "trailing 3")
	got, _ := Classify(Input{Status: "failed", Error: "build: x", LogLines: logs}, Options{MaxLines: 5, MaxBytes: 4000})
	lines := strings.Split(got.LogExcerpt, "\n")
	if len(lines) > 5 {
		t.Errorf("excerpt has %d lines, want at most 5", len(lines))
	}
	if strings.Contains(got.LogExcerpt, "hunter2") || strings.Contains(got.LogExcerpt, "abcdefghijklmnop12345") {
		t.Errorf("excerpt leaked a secret: %q", got.LogExcerpt)
	}
	if strings.Contains(got.LogExcerpt, "step 3 output") {
		t.Errorf("excerpt is not the tail: %q", got.LogExcerpt)
	}

	capped, _ := Classify(Input{Status: "failed", Error: "weird", LogLines: []string{strings.Repeat("x", 500)}}, Options{MaxBytes: 50})
	if len(capped.LogExcerpt) > 50+len("...[truncated 999 bytes]") {
		t.Errorf("excerpt not byte capped: %d", len(capped.LogExcerpt))
	}
}

func TestOptionsFromEnv(t *testing.T) {
	env := map[string]string{EnvExcerptLines: "7", EnvExcerptBytes: "bad"}
	got := OptionsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if got.MaxLines != 7 || got.MaxBytes != defaultExcerptBytes {
		t.Errorf("options = %+v", got)
	}
}
