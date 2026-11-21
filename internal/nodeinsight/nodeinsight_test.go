package nodeinsight

import (
	"errors"
	"path/filepath"
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

func TestBuildDrainProgress(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	mv := func(st, a, b string) DrainMove { return DrainMove{Status: st, StartedAt: a, FinishedAt: b} }
	p := BuildDrainProgress(DrainInput{Status: "running", UpdatedAt: now.Format(time.RFC3339), Moves: []DrainMove{
		mv("succeeded", "2026-11-21T11:00:00Z", "2026-11-21T11:01:00Z"), mv("running", "", ""), mv("running", "", ""),
	}}, now)
	if p.State != DrainRunning || p.Percent != 33 || p.EtaSeconds != 120 {
		t.Fatalf("running: %+v", p)
	}
	old := now.Add(-time.Hour).Format(time.RFC3339)
	if s := BuildDrainProgress(DrainInput{Status: "running", UpdatedAt: old, StallAfter: time.Minute}, now); s.State != DrainStalled {
		t.Fatalf("stalled: %+v", s)
	}
	if e := BuildDrainProgress(DrainInput{Status: "succeeded"}, now); e.Percent != 100 {
		t.Fatalf("empty done: %+v", e)
	}
}

func TestOfflineBufferBoundsAndReplay(t *testing.T) {
	b := NewOfflineBuffer(3, 10)
	for i := 0; i < 5; i++ {
		b.Add(Sample{Kind: "m", Payload: []byte{byte(i), 0}})
	}
	items, bytes, dropped := b.Stats()
	if items != 3 || bytes != 6 || dropped != 2 {
		t.Fatalf("stats %d %d %d", items, bytes, dropped)
	}
	if b.Add(Sample{Payload: make([]byte, 11)}) {
		t.Fatal("oversize sample kept")
	}
	var got []byte
	n := b.Replay(func(s Sample) error {
		if len(got) == 2 {
			return errors.New("down")
		}
		got = append(got, s.Payload[0])
		return nil
	})
	if n != 2 || string(got) != string([]byte{2, 3}) {
		t.Fatalf("replay %d %v", n, got)
	}
	if left, _, _ := b.Stats(); left != 1 {
		t.Fatalf("left %d", left)
	}
	b.SetLimits(0, 0)
	if left, _, _ := b.Stats(); left != 0 {
		t.Fatal("not trimmed")
	}
}

func TestDecideUpdate(t *testing.T) {
	base := UpdateFacts{Channel: "stable", ControlPlane: "1.5.8", AgentVersion: "1.5.6", AgentOnline: true}
	if d := DecideUpdate(base); d.Action != UpdateInstall || d.Target != "1.5.8" {
		t.Fatalf("%+v", d)
	}
	off := base
	off.AgentOnline = false
	if DecideUpdate(off).Action != UpdateHold {
		t.Fatal("offline should hold")
	}
	rb := base
	rb.PreviousGood, rb.HealthFailures, rb.MaxHealthFailures = "1.5.6", 3, 3
	if d := DecideUpdate(rb); d.Action != UpdateRollback || d.Target != "1.5.6" {
		t.Fatalf("%+v", d)
	}
	cur := base
	cur.AgentVersion = "1.5.8"
	if DecideUpdate(cur).Action != UpdateNone {
		t.Fatal("current should be none")
	}
}

func TestVersionHealthRollbackFlow(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	var h VersionHealth
	h.Observe("1.5.6", now)
	h.RecordStable()
	h.Observe("1.5.8", now.Add(time.Hour))
	if h.LastGood != "1.5.6" || h.Failures != 0 {
		t.Fatalf("%+v", h)
	}
	for i := 0; i < 3; i++ {
		h.RecordFailure()
	}
	d := DecideUpdate(UpdateFacts{Channel: "stable", ControlPlane: "1.5.8", AgentVersion: "1.5.8", AgentOnline: true, PreviousGood: h.LastGood, HealthFailures: h.Failures, MaxHealthFailures: 3})
	if d.Action != UpdateRollback || d.Target != "1.5.6" {
		t.Fatalf("%+v", d)
	}
	// A failing version must not replace the good rollback target.
	h.Observe("1.5.9", now.Add(2*time.Hour))
	if h.LastGood != "1.5.6" {
		t.Fatalf("last good %q", h.LastGood)
	}
	path := filepath.Join(t.TempDir(), "health.json")
	if err := h.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := LoadVersionHealth(path)
	if err != nil || back.Current != "1.5.9" || back.LastGood != "1.5.6" {
		t.Fatalf("%+v %v", back, err)
	}
	evs := []Event{{To: "offline", At: now.Add(3 * time.Hour)}, {To: "offline", At: now}, {To: "online", At: now.Add(2 * time.Hour)}}
	if OfflineSince(evs, now.Add(time.Hour)) != 1 {
		t.Fatal("offline count")
	}
}
