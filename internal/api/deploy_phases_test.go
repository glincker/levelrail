package api

import (
	"context"
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

func TestDerivePhasesLiveTimestampedLines(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(s int) time.Time { return t0.Add(time.Duration(s) * time.Second) }
	a := store.DeployAttempt{ID: "d1", Source: store.DeployAttemptSourceUpload, Status: store.DeployAttemptStatusRunning, StartedAt: t0}
	trace := []store.DeployTraceStep{{Step: "building", Status: store.TraceStatusRunning, StartedAt: at(2)}}
	lines := []telemetry.DeployLogEntry{{Timestamp: at(5), Message: "RUN npm ci"}}
	got := derivePhases(a, trace, lines, at(30))
	if got[1].Phase != "install" || got[1].Status != "running" || got[1].StartedAt == nil || got[1].DurationMS == nil || *got[1].DurationMS != 25000 {
		t.Fatalf("install while running: %+v", got[1])
	}
	lines = append(lines, telemetry.DeployLogEntry{Timestamp: at(40), Message: "RUN npm run build"})
	got = derivePhases(a, trace, lines, at(70))
	if got[1].Status != "done" || *got[1].DurationMS != 35000 || got[2].Status != "running" || *got[2].DurationMS != 30000 {
		t.Fatalf("build after install: %+v %+v", got[1], got[2])
	}
	lines = append(lines, telemetry.DeployLogEntry{Timestamp: at(100), Message: "exporting to image"})
	got = derivePhases(a, trace, lines, at(110))
	if got[2].Status != "done" || got[3].Status != "running" || *got[3].DurationMS != 10000 {
		t.Fatalf("export: %+v %+v", got[2], got[3])
	}
}

func TestPhaseSampleCountIsLiveSetting(t *testing.T) {
	rt := &Router{}
	if got := rt.phaseSampleCount(context.Background()); got != 5 {
		t.Fatalf("default %d", got)
	}
	t.Setenv("APP_DEPLOY_PHASES_SAMPLES", "9")
	if got := rt.phaseSampleCount(context.Background()); got != 9 {
		t.Fatalf("env seed %d", got)
	}
	if phasesSamplesCheck("1") == nil || phasesSamplesCheck("51") == nil || phasesSamplesCheck("7") != nil {
		t.Fatal("range check wrong")
	}
}
