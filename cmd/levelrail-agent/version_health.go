package main

import (
	"log/slog"
	"path/filepath"
	"strconv"
	"time"

	"github.com/glincker/levelrail/internal/agentupdate"
	"github.com/glincker/levelrail/internal/nodeinsight"
)

// healthTracker persists which agent version last ran stably, using session
// length as the health signal, so a bad update names the version to return to.
type healthTracker struct {
	path     string
	h        nodeinsight.VersionHealth
	stable   time.Duration
	maxFails int
	// probation is how long after an install a rollback may still be judged.
	probation time.Duration
	current   string
	logger    *slog.Logger
}

func newHealthTracker(dir, current string, stableSeconds int, lookup func(string) (string, bool), logger *slog.Logger) *healthTracker {
	t := &healthTracker{path: filepath.Join(dir, "version-health.json"), stable: time.Duration(stableSeconds) * time.Second, maxFails: 3, probation: 30 * time.Minute, current: current, logger: logger}
	if v, ok := lookup("APP_NODE_AGENT_UPDATE_HEALTH_FAILURES"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			t.maxFails = n
		}
	}
	if v, ok := lookup("APP_NODE_AGENT_UPDATE_PROBATION_SECONDS"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			t.probation = time.Duration(n) * time.Second
		}
	}
	if h, err := nodeinsight.LoadVersionHealth(t.path); err == nil {
		t.h = h
	}
	t.h.Observe(current, time.Now())
	t.save()
	return t
}

func (t *healthTracker) save() {
	if err := t.h.Save(t.path); err != nil {
		t.logger.Warn("agent version health not saved", slog.String("error", err.Error()))
	}
}

// sessionEnded records one finished session and returns the rollback decision.
// handshook is false when the control plane was never reached (a dial failure
// or outage says nothing about this binary), so it is not counted at all.
func (t *healthTracker) sessionEnded(lasted time.Duration, handshook bool) nodeinsight.UpdateDecision {
	none := nodeinsight.UpdateDecision{Action: nodeinsight.UpdateNone, Channel: "local"}
	if !handshook {
		return none
	}
	if lasted >= t.stable {
		t.h.RecordStable()
		agentupdate.ClearMarker(filepath.Dir(t.path))
		t.save()
		return none
	}
	t.h.RecordFailure()
	t.save()
	m, ok := agentupdate.ReadMarker(filepath.Dir(t.path))
	if !ok || !m.InProbation(t.current, t.probation, time.Now()) || t.h.LastGood != m.From {
		return none
	}
	return nodeinsight.DecideUpdate(nodeinsight.UpdateFacts{
		Channel: "local", ControlPlane: t.h.Current, AgentVersion: t.h.Current, AgentOnline: true,
		PreviousGood: t.h.LastGood, HealthFailures: t.h.Failures, MaxHealthFailures: t.maxFails,
	})
}
