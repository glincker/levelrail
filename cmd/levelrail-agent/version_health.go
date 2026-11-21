package main

import (
	"log/slog"
	"path/filepath"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/nodeinsight"
)

// healthTracker persists which agent version last ran stably, using session
// length as the health signal, so a bad update names the version to return to.
type healthTracker struct {
	path     string
	h        nodeinsight.VersionHealth
	stable   time.Duration
	maxFails int
	logger   *slog.Logger
}

func newHealthTracker(dir, current string, stableSeconds int, lookup func(string) (string, bool), logger *slog.Logger) *healthTracker {
	t := &healthTracker{path: filepath.Join(dir, "version-health.json"), stable: time.Duration(stableSeconds) * time.Second, maxFails: 3, logger: logger}
	if v, ok := lookup("APP_NODE_AGENT_UPDATE_HEALTH_FAILURES"); ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			t.maxFails = n
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
func (t *healthTracker) sessionEnded(lasted time.Duration) nodeinsight.UpdateDecision {
	if lasted >= t.stable {
		t.h.RecordStable()
	} else {
		t.h.RecordFailure()
	}
	t.save()
	return nodeinsight.DecideUpdate(nodeinsight.UpdateFacts{
		Channel: "local", ControlPlane: t.h.Current, AgentVersion: t.h.Current, AgentOnline: true,
		PreviousGood: t.h.LastGood, HealthFailures: t.h.Failures, MaxHealthFailures: t.maxFails,
	})
}
