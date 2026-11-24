package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestCollapseLogsFoldsConsecutiveRepeats(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(s int, msg string) telemetry.LogEntry {
		return telemetry.LogEntry{Timestamp: t0.Add(time.Duration(s) * time.Second), Stream: "stdout", Message: msg}
	}
	entries := []telemetry.LogEntry{mk(0, "a"), mk(1, "a"), mk(2, "a"), mk(3, "b"), mk(4, "a")}
	runs := collapseLogs(entries)
	if len(runs) != 3 || runs[0].count != 3 || !runs[0].last.Equal(t0.Add(2*time.Second)) || runs[2].count != 1 {
		t.Fatalf("unexpected runs: %+v", runs)
	}
	res := collapsedResources([]telemetry.LogEntry{runs[0].entry, runs[1].entry}, runs)
	if res[0].Count != 3 || !res[0].First.Equal(t0) || res[1].Count != 0 {
		t.Fatalf("unexpected resources: %+v", res)
	}
}
