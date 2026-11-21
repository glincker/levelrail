package main

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/nodeinsight"
)

func TestHealthTrackerRollsBackAfterShortSessions(t *testing.T) {
	dir := t.TempDir()
	lookup := func(string) (string, bool) { return "", false }
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	old := newHealthTracker(dir, "1.5.6", 60, lookup, log)
	old.sessionEnded(time.Hour)
	cur := newHealthTracker(dir, "1.5.8", 60, lookup, log)
	var d nodeinsight.UpdateDecision
	for i := 0; i < 3; i++ {
		d = cur.sessionEnded(2 * time.Second)
	}
	if d.Action != nodeinsight.UpdateRollback || d.Target != "1.5.6" {
		t.Fatalf("decision %+v", d)
	}
	if d = cur.sessionEnded(time.Hour); d.Action != nodeinsight.UpdateNone {
		t.Fatalf("a stable session should clear it: %+v", d)
	}
}
