package chaos

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestFinishAndSummarize(t *testing.T) {
	ok := Result{Scenario: KillContainer}
	ok.Assert("recovers", true, "")
	ok.Finish()
	bad := Result{Scenario: DiskPressure}
	bad.Assert("fires", false, "no")
	bad.Finish()
	skip := Result{Scenario: OOMDuringReadiness, Status: StatusSkipped}
	skip.Finish()
	s := Summarize("hw", []Result{ok, bad, skip})
	if s.Passed != 1 || s.Failed != 1 || s.Skipped != 1 {
		t.Fatalf("summary = %+v", s)
	}
}

func TestLatestKeepsNewestAndListsUnmeasured(t *testing.T) {
	now := time.Now()
	old := Result{Scenario: KillContainer, RanAt: now.Add(-time.Hour), Summary: "old"}
	cur := Result{Scenario: KillContainer, RanAt: now, Summary: "new"}
	latest, un := Latest([]Result{old, cur})
	if len(latest) != 1 || latest[0].Summary != "new" {
		t.Fatalf("latest = %+v", latest)
	}
	if len(un) != len(Catalog())-1 {
		t.Fatalf("unmeasured = %v", un)
	}
}

func TestWriteTableReportsFailedAssertions(t *testing.T) {
	r := Result{Scenario: KillContainer}
	r.Assert("recovers", false, "timed out")
	r.Finish()
	var b bytes.Buffer
	WriteTable(&b, []Result{r})
	if !strings.Contains(b.String(), "FAILED kill-container: recovers (timed out)") {
		t.Fatalf("table = %q", b.String())
	}
}

func TestResultSetCreatesMetrics(t *testing.T) {
	var r Result
	r.Set("p50_ms", 1.5)
	r.Set("p50_ms", 2.5)
	if len(r.Metrics) != 1 || r.Metrics["p50_ms"] != 2.5 {
		t.Fatalf("metrics = %v", r.Metrics)
	}
}

func TestValid(t *testing.T) {
	if !Valid(OOMDuringReadiness) || Valid("nope") {
		t.Fatal("Valid() wrong")
	}
}
