package api

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

func TestDerivePhasesRunningExport(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	a := store.DeployAttempt{ID: "d1", Source: store.DeployAttemptSourceUpload, Status: store.DeployAttemptStatusRunning, StartedAt: t0}
	logs := []telemetry.DeployLogEntry{
		{Timestamp: at(5), Message: "#5 RUN npm ci"},
		{Timestamp: at(60), Message: "#6 RUN npm run build"},
		{Timestamp: at(200), Message: "#7 exporting to image"},
	}
	trace := []store.DeployTraceStep{{Step: "building", Status: store.TraceStatusRunning, StartedAt: at(3)}}
	got := derivePhases(a, trace, logs, at(300))
	want := map[string]string{"upload": "done", "install": "done", "build": "done", "export_image": "running", "health_check": "pending", "cutover": "pending"}
	for _, p := range got {
		if want[p.Phase] != p.Status {
			t.Errorf("phase %s: status %s, want %s", p.Phase, p.Status, want[p.Phase])
		}
	}
	if len(got) != 6 || got[3].DurationMS == nil || *got[3].DurationMS != 100000 {
		t.Fatalf("export duration wrong: %+v", got)
	}
}

func TestDerivePhasesImageDeployOmitsBuild(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	fin := t0.Add(time.Minute)
	a := store.DeployAttempt{Source: store.DeployAttemptSourceImage, Status: store.DeployAttemptStatusSucceeded, StartedAt: t0, FinishedAt: &fin}
	trace := []store.DeployTraceStep{{Step: "cutover", Status: store.TraceStatusDone, StartedAt: t0.Add(50 * time.Second), EndedAt: fin}}
	got := derivePhases(a, trace, nil, fin)
	if len(got) != 2 || got[0].Phase != "health_check" || got[0].Status != "skipped" || got[1].Status != "done" {
		t.Fatalf("unexpected phases: %+v", got)
	}
}
