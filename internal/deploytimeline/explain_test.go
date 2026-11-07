package deploytimeline

import "testing"

func TestExplainNamesFirstFailedPhase(t *testing.T) {
	tl := Timeline{DeployID: "d1", App: "web", Verdict: VerdictFailed, Events: []Event{
		{Kind: KindBuild, Phase: PhaseBuild, Status: StatusOK, Title: "Build"},
		{Kind: KindLog, Phase: PhaseBuild, Status: StatusInfo, Title: "npm ERR! missing script"},
		{Kind: KindProbe, Phase: PhaseReadiness, Status: StatusFailed, Code: "readiness.probe", Title: "Readiness probe", Reason: "connection refused on :3000"},
		{Kind: KindOutcome, Phase: PhaseOutcome, Status: StatusFailed, Title: "Failed"},
	}}
	ex := Explain(tl, 3)
	if !ex.Failed || ex.Phase != PhaseReadiness || ex.Cause != "connection refused on :3000" || ex.Hint == "" {
		t.Fatalf("unexpected explanation: %+v", ex)
	}
	if len(ex.LastLogs) != 1 || len(ex.Evidence) != 1 {
		t.Fatalf("logs/evidence: %+v", ex)
	}
	if got := Explain(Timeline{Verdict: VerdictSucceeded}, 3); got.Failed {
		t.Fatalf("succeeded deploy reported failed")
	}
}
