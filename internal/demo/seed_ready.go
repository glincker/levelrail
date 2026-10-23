package demo

import (
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type okAttempt struct {
	id string
	at time.Time
}

// readiness stands in for the reconciler the read-only demo never runs: it
// writes the Ready condition and rollout state the dashboard reads, so the
// fleet looks lived in. One app fails after its bad deploy and one is stopped.
func (s *seeder) readiness() error {
	for _, a := range appSpecs {
		ok, has := s.lastOK[a.name]
		if !has {
			return fmt.Errorf("demo: no succeeded attempt for %q", a.name)
		}
		status, reason, msg, since := "True", "Running", fmt.Sprintf("%d/%d replicas ready", a.replicas, a.replicas), ok.at
		switch {
		case a.name == crashloopApp:
			status, reason, since = "False", "CrashLoopBackOff", s.opts.Anchor.Add(-20*time.Minute)
			msg = "container exited 137 (out of memory) 4 times in 5 minutes"
		case a.suspended:
			status, reason, msg, since = "False", "Suspended", "app is stopped", s.at(historyDays/2, 0)
		}
		_, err := s.db.ExecContext(s.ctx, `
			INSERT INTO reconcile_status (controller_name, condition_type, status, reason, message, updated_at)
			VALUES (?, 'Ready', ?, ?, ?, ?)`,
			"application/"+a.name, status, reason, msg, s.stamp(since))
		if err != nil {
			return fmt.Errorf("demo: save condition for %q: %w", a.name, err)
		}
		if a.name == crashloopApp || a.suspended {
			continue
		}
		if err := s.db.SetDeployAttemptRollout(s.ctx, ok.id, store.RolloutStateServing); err != nil {
			return fmt.Errorf("demo: set rollout for %q: %w", a.name, err)
		}
	}
	return nil
}
