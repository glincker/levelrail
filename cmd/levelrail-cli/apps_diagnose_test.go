package main

import (
	"strings"
	"testing"
)

func TestRunAppsDiagnose_JSON(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, diagnosisResource{
		Explanation:     "Docker could not pull or find the container image.",
		Suggestion:      "Check the image name and tag.",
		Confidence:      "high",
		MatchedSignals:  []diagnosisSignal{{Source: "deploy_attempt.error", Excerpt: "pull access denied"}},
		DeployAttemptID: "dep_1",
	})
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"apps", "diagnose", "web", "--api-url", srv.URL, "--json"})
	if !strings.Contains(stdout, `"confidence": "high"`) {
		t.Errorf("stdout = %q, want it to contain the confidence field", stdout)
	}
	if gotPath != "/api/v1/apps/web/diagnose" {
		t.Errorf("request path = %q, want /api/v1/apps/web/diagnose", gotPath)
	}
}

func TestRunAppsDiagnose_DeployFlag(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, diagnosisResource{Confidence: "none"})
	t.Cleanup(srv.Close)

	runCLIExpectOK(t, []string{"apps", "diagnose", "web", "--deploy", "dep_old", "--api-url", srv.URL, "--json"})
	if gotPath != "/api/v1/apps/web/diagnose" {
		t.Errorf("request path = %q, want /api/v1/apps/web/diagnose", gotPath)
	}
}

func TestRunAppsDiagnose_Human(t *testing.T) {
	srv := newListEchoServer(t, nil, diagnosisResource{
		Explanation: "The container could not bind to its configured port.",
		Suggestion:  "Free up the port or change app.yaml.",
		Confidence:  "high",
		MatchedSignals: []diagnosisSignal{
			{Source: "deploy_attempt.error", Excerpt: "port is already allocated"},
		},
	})
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"apps", "diagnose", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "could not bind to its configured port") {
		t.Errorf("stdout = %q, want the explanation text", stdout)
	}
	if !strings.Contains(stdout, "port is already allocated") {
		t.Errorf("stdout = %q, want the matched signal excerpt", stdout)
	}
}

func TestRunAppsDiagnose_MissingName(t *testing.T) {
	var stdout, stderr strings.Builder
	got := run("levelrail-cli-test", []string{"apps", "diagnose"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}
