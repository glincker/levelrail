package stuck

import (
	"testing"
	"time"
)

func TestExplain(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	notReady := func(reason, msg string, ago time.Duration) []Condition {
		return []Condition{{Type: "Ready", Status: "False", Reason: reason, Message: msg, Since: now.Add(-ago)}}
	}
	tests := []struct {
		name      string
		in        Input
		wantTop   string
		wantStuck bool
		wantFix   string
	}{
		{"healthy", Input{Kind: KindApp, Conditions: []Condition{{Type: "Ready", Status: "True", Reason: "Running", Since: now}}}, "", false, ""},
		{"image pull by reason", Input{Kind: KindApp, Conditions: notReady("ImagePullFailed", "denied", 20*time.Minute)}, CauseImagePull, true, ""},
		{"disk critical beats others", Input{Kind: KindApp, Conditions: notReady("CreateFailed", "x", time.Minute), Ctx: Context{DiskCritical: true, DiskDetail: "/var/lib/docker 2% free"}}, CauseDiskFull, false, "disk.reclaim"},
		{"node offline by context", Input{Kind: KindDatabase, Conditions: notReady("StartFailed", "x", time.Hour), Ctx: Context{NodeStatus: "offline"}}, CauseNodeOffline, true, ""},
		{"port conflict by text only", Input{Kind: KindApp, Conditions: notReady("StartFailed", "bind: address already in use", time.Hour)}, CausePortConflict, true, ""},
		{"readiness", Input{Kind: KindApp, Conditions: notReady("ReadinessFailed", "", 2*time.Minute)}, CauseReadiness, false, ""},
		{"dependency by context", Input{Kind: KindApp, Conditions: notReady("Pending", "", time.Hour), Ctx: Context{DependencyNotReady: "database/main"}}, CauseDependency, true, ""},
		{"placement by text", Input{Kind: KindApp, Conditions: notReady("Pending", "no schedulable node for memory 64Gi", time.Hour)}, CausePlacement, true, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.in.Now, tc.in.Threshold = now, 10*time.Minute
			rep := Explain(tc.in)
			if tc.wantTop == "" {
				if !rep.Ready || len(rep.Causes) != 0 {
					t.Fatalf("want healthy, got %+v", rep)
				}
				return
			}
			if len(rep.Causes) == 0 || rep.Causes[0].Code != tc.wantTop {
				t.Fatalf("top cause = %+v, want %s", rep.Causes, tc.wantTop)
			}
			if len(rep.Causes[0].Evidence) == 0 {
				t.Fatalf("cause %s has no evidence", tc.wantTop)
			}
			if rep.Stuck != tc.wantStuck {
				t.Fatalf("stuck = %v, want %v", rep.Stuck, tc.wantStuck)
			}
			if tc.wantFix != "" {
				found := false
				for _, a := range rep.Causes[0].Actions {
					found = found || (a.Type == ActionFix && a.FixID == tc.wantFix)
				}
				if !found {
					t.Fatalf("missing fix action %s in %+v", tc.wantFix, rep.Causes[0].Actions)
				}
			}
		})
	}
}
