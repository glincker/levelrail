package api

import (
	"testing"
	"time"

	"github.com/glincker/levelrail/internal/deploy"
	"github.com/glincker/levelrail/internal/store"
)

func TestVacationViewReportsDaysLeftAndLifts(t *testing.T) {
	start := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	ws := []store.DeployFreezeWindow{deploy.VacationWindow(start, 3, "away")}
	if v := vacationView(ws, start.Add(24*time.Hour)); !v.Active || v.DaysLeft != 2 || v.Reason != "away" {
		t.Fatalf("day two: %+v", v)
	}
	if v := vacationView(ws, start.Add(73*time.Hour)); v.Active {
		t.Fatalf("after end: %+v", v)
	}
}
