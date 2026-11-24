package deploywhy

import "testing"

func TestComputeVerdictPriorityAndNewTemplates(t *testing.T) {
	two := 2
	base := []Template{{TemplateID: "a", Count: 5}}
	cur := []Template{{TemplateID: "a", Count: 1}, {TemplateID: "b", Count: 3}, {TemplateID: "c", Count: 9}}
	r := Compute(Input{DeployID: "d", Failed: true, ExitCode: &two, Templates: cur, BaselineTemplate: base})
	if r.Verdict != VerdictExitNonZero || r.NewTemplateCount != 2 || r.NewTemplates[0].TemplateID != "c" {
		t.Fatalf("unexpected report: %+v", r)
	}
	r = Compute(Input{Failed: true, Health: &Health{Success: false}, Templates: cur})
	if r.Verdict != VerdictHealthFailed {
		t.Fatalf("health verdict = %q", r.Verdict)
	}
	if got := Compute(Input{Failed: true, Templates: cur, BaselineTemplate: base}).Verdict; got != VerdictNewLogs {
		t.Fatalf("logs verdict = %q", got)
	}
	if got := Compute(Input{Failed: true}).Verdict; got != VerdictNoSignal {
		t.Fatalf("no signal verdict = %q", got)
	}
	if got := Compute(Input{}).Verdict; got != VerdictOK {
		t.Fatalf("ok verdict = %q", got)
	}
}
