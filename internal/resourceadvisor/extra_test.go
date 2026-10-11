package resourceadvisor

import (
	"testing"
	"time"
)

func TestDatabaseCPUFloorAndHeldWindow(t *testing.T) {
	th := DefaultThresholds()
	r := Resource{Ref: db("main"), MemoryLimit: 512 * mib,
		Memory:     series(3*day, 5*time.Minute, 0.05, flat(300*mib)),
		CPUPercent: series(3*day, 5*time.Minute, 0.5, flat(1))}
	f := kinds(limitFindings(r, th, testNow))[KindCPULimitMissing]
	if f.Action == nil || f.Action.Value != th.DatabaseCPUFloor {
		t.Fatalf("database cpu suggestion = %+v, want the %d floor", f.Action, th.DatabaseCPUFloor)
	}
	if f.Evidence.Window != "last 3 days" {
		t.Fatalf("window = %q, want the history actually held", f.Evidence.Window)
	}
}

func TestCapacityNoteWithoutNodeRecord(t *testing.T) {
	got := hostFindings(Node{ID: "local", NoIdentity: true, MemoryTotal: 8 * gib}, nil, DefaultThresholds(), testNow)
	if len(got) != 1 || got[0].Kind != KindCapacityUnknown {
		t.Fatalf("got %v, want a capacity note even when host memory was read directly", got)
	}
}
