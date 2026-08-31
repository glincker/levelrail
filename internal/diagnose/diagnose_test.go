package diagnose

import (
	"strings"
	"testing"
)

func TestDiagnose_Signatures(t *testing.T) {
	tests := []struct {
		name                  string
		in                    Input
		wantExplanationSubstr string
		wantConfidence        string
		wantSignalSource      string
	}{
		{
			name:                  "docker daemon unreachable",
			in:                    Input{Attempt: &AttemptInput{Status: "failed", Error: "deploy: service \"web\": create \"web-abc\": Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"}},
			wantExplanationSubstr: "Docker daemon",
			wantConfidence:        ConfidenceHigh,
			wantSignalSource:      "deploy_attempt.error",
		},
		{
			name:                  "image pull failure",
			in:                    Input{Attempt: &AttemptInput{Status: "failed", Error: "deploy: service \"web\": create \"web-abc\": Error response from daemon: pull access denied for levelrail/web, repository does not exist or may require 'docker login'"}},
			wantExplanationSubstr: "pull or find the container image",
			wantConfidence:        ConfidenceHigh,
			wantSignalSource:      "deploy_attempt.error",
		},
		{
			name:                  "missing dockerfile",
			in:                    Input{Attempt: &AttemptInput{Status: "failed", Error: "deploy: service \"web\": build: failed to read dockerfile: open Dockerfile: no such file or directory"}},
			wantExplanationSubstr: "could not find a Dockerfile",
			wantConfidence:        ConfidenceHigh,
		},
		{
			name:                  "dependency install failure",
			in:                    Input{Attempt: &AttemptInput{Status: "failed", Error: "deploy: service \"web\": build: failed to solve: process \"/bin/sh -c npm ci\" did not complete successfully: exit code: 1"}},
			wantExplanationSubstr: "dependency installation step",
			wantConfidence:        ConfidenceMedium,
		},
		{
			name:                  "port conflict",
			in:                    Input{Attempt: &AttemptInput{Status: "failed", Error: "deploy: service \"web\": create \"web-abc\": Bind for 0.0.0.0:8080 failed: port is already allocated"}},
			wantExplanationSubstr: "already using it",
			wantConfidence:        ConfidenceHigh,
		},
		{
			name:                  "health check timeout",
			in:                    Input{Conditions: []ConditionInput{{Type: "Ready", Status: "False", Reason: "ReadinessFailed", Message: "readiness probe: timed out waiting for a successful response"}}},
			wantExplanationSubstr: "never passed its readiness health check",
			wantConfidence:        ConfidenceHigh,
			wantSignalSource:      "condition:Ready.ReadinessFailed",
		},
		{
			name:                  "oom kill from logs",
			in:                    Input{RecentLogLines: []string{"starting server", "container was OOMKilled by the kernel"}},
			wantExplanationSubstr: "using too much memory",
			wantConfidence:        ConfidenceMedium,
			wantSignalSource:      "logs",
		},
		{
			name:                  "crashloop firing with no text match",
			in:                    Input{Crashloop: &CrashloopInput{Firing: true, RestartCount: 6, RestartCountThreshold: 5, RestartWindow: "5m0s"}},
			wantExplanationSubstr: "repeatedly restarting",
			wantConfidence:        ConfidenceHigh,
			wantSignalSource:      "crashloop_rule",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Diagnose(tt.in)
			if !strings.Contains(got.Explanation, tt.wantExplanationSubstr) {
				t.Errorf("Explanation = %q, want substring %q", got.Explanation, tt.wantExplanationSubstr)
			}
			if got.Confidence != tt.wantConfidence {
				t.Errorf("Confidence = %q, want %q", got.Confidence, tt.wantConfidence)
			}
			if got.Suggestion == "" {
				t.Error("Suggestion is empty, want a non-empty suggested action")
			}
			if len(got.MatchedSignals) == 0 {
				t.Fatal("MatchedSignals is empty, want at least one signal backing the explanation")
			}
			if tt.wantSignalSource != "" && got.MatchedSignals[0].Source != tt.wantSignalSource {
				t.Errorf("MatchedSignals[0].Source = %q, want %q", got.MatchedSignals[0].Source, tt.wantSignalSource)
			}
			if got.MatchedSignals[0].Excerpt == "" {
				t.Error("MatchedSignals[0].Excerpt is empty, want the literal matched text")
			}
		})
	}
}

func TestDiagnose_PriorityOrder(t *testing.T) {
	// Both docker-daemon-unreachable and image-pull-failure patterns are
	// present; docker daemon unreachable is declared first (more
	// fundamental: nothing about the image can even be evaluated), so it
	// must win as the primary explanation.
	in := Input{Attempt: &AttemptInput{
		Status: "failed",
		Error:  "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. pull access denied for levelrail/web",
	}}
	got := Diagnose(in)
	if !strings.Contains(got.Explanation, "Docker daemon") {
		t.Errorf("Explanation = %q, want the docker-daemon-unreachable explanation to win priority", got.Explanation)
	}
	if len(got.MatchedSignals) != 2 {
		t.Errorf("MatchedSignals count = %d, want 2 (both signatures recorded as evidence)", len(got.MatchedSignals))
	}
}

func TestDiagnose_CrashloopIsLowestPriority(t *testing.T) {
	// A crashloop firing alongside a more specific OOM signal: the OOM
	// explanation is more useful (it explains *why* the loop is
	// happening), so it must be primary, not the generic crashloop text.
	in := Input{
		Attempt:   &AttemptInput{Status: "failed", Error: "container exited with exit code 137"},
		Crashloop: &CrashloopInput{Firing: true, RestartCount: 6, RestartCountThreshold: 5, RestartWindow: "5m0s"},
	}
	got := Diagnose(in)
	if !strings.Contains(got.Explanation, "using too much memory") {
		t.Errorf("Explanation = %q, want the OOM explanation to win over the generic crashloop one", got.Explanation)
	}
	if len(got.MatchedSignals) != 2 {
		t.Errorf("MatchedSignals count = %d, want 2 (OOM signature plus the crashloop rule)", len(got.MatchedSignals))
	}
}

func TestDiagnose_FallbackWithRawSignal(t *testing.T) {
	in := Input{Attempt: &AttemptInput{Status: "failed", Error: "something unusual and not covered by any known pattern happened"}}
	got := Diagnose(in)
	if got.Confidence != ConfidenceNone {
		t.Errorf("Confidence = %q, want %q", got.Confidence, ConfidenceNone)
	}
	if len(got.MatchedSignals) != 1 || got.MatchedSignals[0].Source != "deploy_attempt.error" {
		t.Fatalf("MatchedSignals = %+v, want the raw attempt error surfaced as a signal", got.MatchedSignals)
	}
	if !strings.Contains(got.MatchedSignals[0].Excerpt, "something unusual") {
		t.Errorf("Excerpt = %q, want it to contain the raw error text", got.MatchedSignals[0].Excerpt)
	}
}

func TestDiagnose_FallbackWithFailingCondition(t *testing.T) {
	in := Input{Conditions: []ConditionInput{{Type: "Ready", Status: "False", Reason: "SomeUnknownReason", Message: "an error this engine has no pattern for"}}}
	got := Diagnose(in)
	if got.Confidence != ConfidenceNone {
		t.Errorf("Confidence = %q, want %q", got.Confidence, ConfidenceNone)
	}
	if len(got.MatchedSignals) != 1 || got.MatchedSignals[0].Source != "condition:Ready" {
		t.Fatalf("MatchedSignals = %+v, want the failing condition surfaced as a signal", got.MatchedSignals)
	}
}

func TestDiagnose_NothingFailed(t *testing.T) {
	got := Diagnose(Input{ServiceName: "web"})
	if got.Confidence != ConfidenceNone {
		t.Errorf("Confidence = %q, want %q", got.Confidence, ConfidenceNone)
	}
	if len(got.MatchedSignals) != 0 {
		t.Errorf("MatchedSignals = %+v, want none: nothing failed", got.MatchedSignals)
	}
	if got.Explanation == "" || got.Suggestion == "" {
		t.Error("even the healthy-app fallback should have a non-empty Explanation and Suggestion")
	}
}

func TestDiagnose_SucceededAttemptNeverMatchesFallbackSignal(t *testing.T) {
	// A succeeded attempt's Error field is always empty by construction
	// elsewhere in this codebase, but this asserts the fallback path
	// itself never surfaces a non-failed attempt as though it were
	// evidence of a failure.
	in := Input{Attempt: &AttemptInput{Status: "succeeded", Error: ""}}
	got := Diagnose(in)
	if len(got.MatchedSignals) != 0 {
		t.Errorf("MatchedSignals = %+v, want none for a succeeded attempt with no error", got.MatchedSignals)
	}
}

func TestDiagnose_LogExcerptIsOnlyTheMatchingLine(t *testing.T) {
	in := Input{RecentLogLines: []string{
		"line one: nothing interesting",
		"line two: also nothing interesting",
		"line three: process killed, exit code 137",
		"line four: cleanup",
	}}
	got := Diagnose(in)
	if len(got.MatchedSignals) != 1 {
		t.Fatalf("MatchedSignals = %+v, want exactly one", got.MatchedSignals)
	}
	excerpt := got.MatchedSignals[0].Excerpt
	if !strings.Contains(excerpt, "exit code 137") {
		t.Errorf("Excerpt = %q, want it to contain the matching line", excerpt)
	}
	if strings.Contains(excerpt, "line one") || strings.Contains(excerpt, "line four") {
		t.Errorf("Excerpt = %q, want only the matching line quoted, not the whole log window", excerpt)
	}
}

func TestDiagnose_ExcerptTruncation(t *testing.T) {
	longErr := "pull access denied for levelrail/web: " + strings.Repeat("x", 1000)
	got := Diagnose(Input{Attempt: &AttemptInput{Status: "failed", Error: longErr}})
	if len(got.MatchedSignals) != 1 {
		t.Fatalf("MatchedSignals = %+v, want exactly one", got.MatchedSignals)
	}
	if len(got.MatchedSignals[0].Excerpt) > maxExcerptLen+len("...") {
		t.Errorf("Excerpt length = %d, want capped near %d", len(got.MatchedSignals[0].Excerpt), maxExcerptLen)
	}
}

func TestDiagnose_Deterministic(t *testing.T) {
	in := Input{Attempt: &AttemptInput{Status: "failed", Error: "port is already allocated"}}
	first := Diagnose(in)
	second := Diagnose(in)
	if !equalResults(first, second) {
		t.Errorf("Diagnose(in) is not deterministic: %+v vs %+v", first, second)
	}
}

func equalResults(a, b Result) bool {
	if a.Explanation != b.Explanation || a.Suggestion != b.Suggestion || a.Confidence != b.Confidence {
		return false
	}
	if len(a.MatchedSignals) != len(b.MatchedSignals) {
		return false
	}
	for i := range a.MatchedSignals {
		if a.MatchedSignals[i] != b.MatchedSignals[i] {
			return false
		}
	}
	return true
}
