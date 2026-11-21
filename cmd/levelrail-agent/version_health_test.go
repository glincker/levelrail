package main

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agentupdate"
	"github.com/GLINCKER/levelrail/internal/nodeinsight"
)

func TestHealthTrackerRollsBackAfterShortSessions(t *testing.T) {
	dir := t.TempDir()
	lookup := func(string) (string, bool) { return "", false }
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	old := newHealthTracker(dir, "1.5.6", 60, lookup, log)
	old.sessionEnded(time.Hour, true)
	if err := agentupdate.WriteMarker(dir, agentupdate.Marker{From: "1.5.6", To: "1.5.8", InstalledAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	cur := newHealthTracker(dir, "1.5.8", 60, lookup, log)
	var d nodeinsight.UpdateDecision
	for i := 0; i < 3; i++ {
		d = cur.sessionEnded(2*time.Second, true)
	}
	if d.Action != nodeinsight.UpdateRollback || d.Target != "1.5.6" {
		t.Fatalf("decision %+v", d)
	}
	if d = cur.sessionEnded(time.Hour, true); d.Action != nodeinsight.UpdateNone {
		t.Fatalf("a stable session should clear it: %+v", d)
	}
}

func newTracker(t *testing.T, dir, ver string) *healthTracker {
	t.Helper()
	return newHealthTracker(dir, ver, 60, func(string) (string, bool) { return "", false }, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestOutageAfterStableSessionNeverRollsBack(t *testing.T) {
	dir := t.TempDir()
	newTracker(t, dir, "1.5.6").sessionEnded(time.Hour, true)
	if err := agentupdate.WriteMarker(dir, agentupdate.Marker{From: "1.5.6", To: "1.5.8", InstalledAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	cur := newTracker(t, dir, "1.5.8")
	cur.sessionEnded(time.Hour, true) // first stable session clears the marker
	if _, ok := agentupdate.ReadMarker(dir); ok {
		t.Fatal("marker must be cleared after a stable session")
	}
	for i := 0; i < 6; i++ {
		if d := cur.sessionEnded(time.Second, true); d.Action == nodeinsight.UpdateRollback {
			t.Fatalf("a later outage rolled back: %+v", d)
		}
	}
}

func TestFailedDialsAreNotCounted(t *testing.T) {
	dir := t.TempDir()
	newTracker(t, dir, "1.5.6").sessionEnded(time.Hour, true)
	if err := agentupdate.WriteMarker(dir, agentupdate.Marker{From: "1.5.6", To: "1.5.8", InstalledAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	cur := newTracker(t, dir, "1.5.8")
	for i := 0; i < 10; i++ {
		if d := cur.sessionEnded(time.Second, false); d.Action != nodeinsight.UpdateNone {
			t.Fatalf("dial failure counted: %+v", d)
		}
	}
}

func TestNoRollbackOutsideProbationOrForOtherVersion(t *testing.T) {
	for name, m := range map[string]agentupdate.Marker{
		"expired":       {From: "1.5.6", To: "1.5.8", InstalledAt: time.Now().Add(-24 * time.Hour)},
		"other version": {From: "1.5.6", To: "1.5.9", InstalledAt: time.Now()},
		"other from":    {From: "1.4.0", To: "1.5.8", InstalledAt: time.Now()},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			newTracker(t, dir, "1.5.6").sessionEnded(time.Hour, true)
			if err := agentupdate.WriteMarker(dir, m); err != nil {
				t.Fatal(err)
			}
			cur := newTracker(t, dir, "1.5.8")
			for i := 0; i < 4; i++ {
				if d := cur.sessionEnded(time.Second, true); d.Action == nodeinsight.UpdateRollback {
					t.Fatalf("unexpected rollback: %+v", d)
				}
			}
		})
	}
}
