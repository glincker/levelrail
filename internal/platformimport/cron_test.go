package platformimport

import "testing"

func TestNormalizeScheduleAndJobName(t *testing.T) {
	for in, want := range map[string]string{"hourly": "0 * * * *", "@daily": "0 0 * * *", "weekly": "0 0 * * 0", "*/5 * * * *": "*/5 * * * *"} {
		if got := NormalizeSchedule(in); got != want {
			t.Errorf("NormalizeSchedule(%q) = %q, want %q", in, got, want)
		}
	}
	for in, want := range map[string]string{"Nightly cleanup": "nightly-cleanup", "  ": "task", "_x_": "x"} {
		if got := SanitizeJobName(in); got != want {
			t.Errorf("SanitizeJobName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPlanCarriesCronsAsJobs(t *testing.T) {
	d := &Discovery{Platform: Coolify, Apps: []App{{SourceID: "a", Name: "web", Kind: SourceImage, Image: "nginx:1", Port: 80,
		Crons: []Cron{{Name: "Nightly Cleanup", Schedule: "daily", Command: "run"}}}}}
	p := BuildPlan(d, PlanOptions{})
	if len(p.Apps) != 1 || len(p.Apps[0].Crons) != 1 {
		t.Fatalf("apps: %+v", p.Apps)
	}
	if c := p.Apps[0].Crons[0]; c.Name != "nightly-cleanup" || c.Schedule != "0 0 * * *" || c.Command != "run" {
		t.Errorf("cron: %+v", c)
	}
	if p.Report.Items[0].Status != StatusMapped {
		t.Errorf("a cron alone must not need attention: %+v", p.Report.Items[0])
	}
}
