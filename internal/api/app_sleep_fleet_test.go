package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestBuildSleepFleetCountsAndReclaimedMemory(t *testing.T) {
	mem := int64(256 << 20)
	res := &store.ServiceResources{MemoryBytes: mem}
	svcs := []store.DesiredService{
		{Name: "idle", Resources: res},
		{Name: "busy", Resources: res},
		{Name: "hidden", Resources: res},
	}
	now := time.Now()
	rows := []store.AppSleep{
		{ServiceName: "busy", IdleMinutes: 30},
		{ServiceName: "idle", IdleMinutes: 30, Sleeping: true},
		{ServiceName: "hidden", IdleMinutes: 30, Sleeping: true, Since: now},
		{ServiceName: "off", IdleMinutes: 0},
	}
	got := buildSleepFleet(rows, svcs, func(name string) bool { return name != "hidden" })
	if got.Enabled != 2 || got.Sleeping != 1 || got.ReclaimedBytes != mem {
		t.Fatalf("fleet = %+v, want 2 enabled, 1 asleep, %d reclaimed", got, mem)
	}
	if len(got.Apps) != 2 || got.Apps[0].Name != "idle" {
		t.Errorf("apps = %+v, want the sleeping app first and hidden/disabled omitted", got.Apps)
	}
}
