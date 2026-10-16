package main

import (
	"strings"
	"testing"
)

func TestDeployStepsFinal(t *testing.T) {
	tests := []struct {
		name       string
		ev         deployStepEvent
		wantFinal  bool
		wantFailed bool
	}{
		{"summary done ends the trace", deployStepEvent{Step: "deploy_summary", Status: "done"}, true, false},
		{"summary failed ends it as a failure", deployStepEvent{Step: "deploy_summary", Status: "failed"}, true, true},
		{"a failed reconcile step alone is retried, not final", deployStepEvent{Step: "readiness_gate", Status: "failed"}, false, false},
		{"a failed pull alone is not final", deployStepEvent{Step: "image_pull", Status: "failed"}, false, false},
		{"a build failure is final", deployStepEvent{Step: "building", Status: "failed"}, true, true},
		{"deploying done waits for the summary", deployStepEvent{Step: "deploying", Status: "done"}, false, false},
		{"running probe is not final", deployStepEvent{Step: "readiness_probe", Status: "warning"}, false, false},
	}
	for _, tt := range tests {
		final, failed := deployStepsFinal(tt.ev)
		if final != tt.wantFinal || failed != tt.wantFailed {
			t.Errorf("%s: final=%v failed=%v, want %v/%v", tt.name, final, failed, tt.wantFinal, tt.wantFailed)
		}
	}
}

func TestFormatStepLine(t *testing.T) {
	got := formatStepLine(deployStepEvent{
		Timestamp: "2026-10-16T10:00:01Z", Step: "image_pull", Status: "failed", Reason: "ImagePullFailed",
		Detail: "unexpected EOF", DurationMS: 1500,
	})
	for _, want := range []string{"image_pull", "failed", "1.5s", "[ImagePullFailed]", "unexpected EOF"} {
		if !strings.Contains(got, want) {
			t.Errorf("line %q missing %q", got, want)
		}
	}
	keyed := formatStepLine(deployStepEvent{Step: "readiness_probe", Key: "readiness_probe.2", Service: "web", Status: "warning"})
	if !strings.Contains(keyed, "web/readiness_probe.2") {
		t.Errorf("line %q should show service and key", keyed)
	}
}
