package nodeinsight

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/nodepreflight"
)

func TestBuildTimeline(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	events := []Event{
		{From: "online", To: "offline", At: now.Add(-3 * time.Hour)},
		{From: "offline", To: "online", At: now.Add(-1 * time.Hour)},
	}
	tl := BuildTimeline(events, "online", 4*time.Hour, now)
	if len(tl.Segments) != 3 || tl.Transitions != 2 {
		t.Fatalf("segments %d transitions %d", len(tl.Segments), tl.Transitions)
	}
	if tl.Segments[0].State != "online" || tl.Segments[1].State != "offline" || tl.Segments[1].Seconds != 7200 {
		t.Fatalf("unexpected segments: %+v", tl.Segments)
	}
	if tl.UptimePercent == nil || *tl.UptimePercent != 50 {
		t.Fatalf("uptime %v", tl.UptimePercent)
	}
	quiet := BuildTimeline(nil, "pending", time.Hour, now)
	if len(quiet.Segments) != 1 || quiet.UptimePercent != nil {
		t.Fatalf("pending timeline: %+v", quiet)
	}
}

func TestBuildSkewOrdersPlan(t *testing.T) {
	rep := BuildSkew("v1.5.8", []NodeVersion{
		{ID: "a", Name: "a", Version: "v1.5.0", Online: false, Schedulable: true},
		{ID: "b", Name: "b", Version: "v1.4.0", Online: true, Schedulable: true},
		{ID: "c", Name: "c", Version: "v1.3.0", Online: true, Schedulable: false},
		{ID: "d", Name: "d", Version: "v1.5.8", Online: true, Schedulable: true},
		{ID: "e", Name: "e", Online: true, Schedulable: true},
	})
	if rep.Behind != 3 || rep.Current != 1 || rep.Unknown != 1 {
		t.Fatalf("counts: %+v", rep)
	}
	got := []string{rep.Plan[0].NodeID, rep.Plan[1].NodeID, rep.Plan[2].NodeID}
	if got[0] != "c" || got[1] != "b" || got[2] != "a" {
		t.Fatalf("order %v", got)
	}
}

func TestExtraChecks(t *testing.T) {
	now := time.Now()
	days := 3
	seen := now.Add(-time.Minute)
	checks := ExtraChecks(DoctorFacts{Online: true, LastSeen: &seen, CertState: "expiring", CertDaysLeft: &days, AgentVersion: "v1.0.0", ControlPlane: "v1.5.8", Schedulable: false}, now)
	want := map[string]string{CheckHeartbeat: nodepreflight.StatusPass, CheckCert: nodepreflight.StatusWarn, CheckAgentVersion: nodepreflight.StatusWarn, CheckSchedulable: nodepreflight.StatusWarn}
	for _, c := range checks {
		if want[c.ID] != c.Status {
			t.Errorf("%s: got %s want %s", c.ID, c.Status, want[c.ID])
		}
		delete(want, c.ID)
	}
	if len(want) != 0 {
		t.Fatalf("missing checks %v", want)
	}
}
