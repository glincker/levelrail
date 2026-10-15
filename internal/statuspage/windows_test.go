package statuspage

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeWindows []store.MaintenanceWindow

func (f fakeWindows) ListMaintenanceWindows(context.Context, time.Time) ([]store.MaintenanceWindow, error) {
	return f, nil
}

func TestWithWindowsListsMaintenanceWithoutTarget(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	s := &Service{windows: fakeWindows{
		{ID: "mw_1", Title: "Upcoming", Scope: store.MaintenanceScopeApp, Target: "secret-app",
			StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour), Message: "details"},
		{ID: "mw_2", Title: "Now", Scope: store.MaintenanceScopeAll, StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour)},
	}}
	v := s.withWindows(context.Background(), View{Components: []ComponentView{{Name: "web", Status: Operational}}}, now)

	if len(v.Maintenance) != 2 {
		t.Fatalf("maintenance = %d entries, want 2", len(v.Maintenance))
	}
	if v.Maintenance[0].Status != StatusScheduled || v.Maintenance[1].Status != StatusInProgress {
		t.Fatalf("statuses = %s, %s", v.Maintenance[0].Status, v.Maintenance[1].Status)
	}
	if v.Components[0].Status != Maintenance || v.Status != Maintenance {
		t.Fatalf("an active all-scope window must mark components, got %s / %s", v.Components[0].Status, v.Status)
	}
}
