package alerting

import (
	"strings"
	"testing"
)

func TestSummaryDeployText_FailedIncludesLikelyCause(t *testing.T) {
	got := summaryDeployText(DeployOutcome{AppName: "web", Image: "web:1", Error: "timed out waiting for readiness"})
	if !strings.Contains(got, "Likely cause:") {
		t.Fatalf("want a diagnosis line, got %q", got)
	}
}

func TestSummaryDeployText_NoMatchOmitsLikelyCause(t *testing.T) {
	got := summaryDeployText(DeployOutcome{AppName: "web", Image: "web:1", Error: "qqq"})
	if strings.Contains(got, "Likely cause:") {
		t.Fatalf("unexpected diagnosis in %q", got)
	}
}

func TestSummaryDeployText_SuccessHasNoDiagnosis(t *testing.T) {
	got := summaryDeployText(DeployOutcome{AppName: "web", Image: "web:1", Succeeded: true})
	if strings.Contains(got, "Likely cause:") {
		t.Fatalf("unexpected diagnosis in %q", got)
	}
}
