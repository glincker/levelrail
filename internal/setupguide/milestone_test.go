package setupguide

import "testing"

func TestFirstAppMilestone(t *testing.T) {
	g := func(done ...StepID) Guide {
		var steps []Step
		set := map[StepID]bool{}
		for _, d := range done {
			set[d] = true
		}
		for _, id := range StepIDs() {
			st := StateTodo
			if set[id] {
				st = StateDone
			}
			steps = append(steps, Step{ID: id, State: st})
		}
		return Guide{Steps: steps}
	}
	cases := []struct {
		name      string
		guide     Guide
		cur, next Milestone
	}{
		{"empty guide", Guide{}, MilestoneInstallDone, MilestoneHTTPSOn},
		{"fresh", g(), MilestoneInstallDone, MilestoneHTTPSOn},
		{"https", g(StepTLS), MilestoneHTTPSOn, MilestoneFirstDomain},
		{"gap stops order", g(StepTLS, StepFirstApp), MilestoneHTTPSOn, MilestoneFirstDomain},
		{"app", g(StepTLS, StepAppDomain, StepFirstApp), MilestoneFirstAppDeployed, MilestoneFirstBackup},
		{"all", g(StepTLS, StepAppDomain, StepFirstApp, StepFirstBackup, StepVerifiedBackup), MilestoneRestoreVerified, ""},
	}
	for _, c := range cases {
		cur, next := FirstAppMilestone(c.guide)
		if cur != c.cur || next != c.next {
			t.Errorf("%s: got %s/%s want %s/%s", c.name, cur, next, c.cur, c.next)
		}
	}
	na := g(StepAppDomain)
	for i := range na.Steps {
		if na.Steps[i].ID == StepTLS {
			na.Steps[i].State = StateNotApplicable
		}
	}
	if cur, _ := FirstAppMilestone(na); cur != MilestoneFirstDomain {
		t.Errorf("not applicable tls: got %s", cur)
	}
}

func TestDeriveSetsMilestone(t *testing.T) {
	gd := Derive(Input{})
	if gd.Milestone != MilestoneInstallDone || gd.NextMilestone == "" {
		t.Errorf("got %q/%q", gd.Milestone, gd.NextMilestone)
	}
}
