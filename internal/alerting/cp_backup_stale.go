package alerting

import (
	"fmt"
	"time"
)

// DefaultControlPlaneBackupMaxAge matches the doctor check's warning age.
const DefaultControlPlaneBackupMaxAge = 3 * 24 * time.Hour

// ControlPlaneBackupSource reports when the newest control plane snapshot was taken.
type ControlPlaneBackupSource interface {
	Newest() (time.Time, bool, error)
}

// EvaluateControlPlaneBackupStale runs one KindControlPlaneBackupStale rule.
// It fires when the newest snapshot is older than the rule's ForDuration
// (else defaultMaxAge). No snapshot yet stays quiet, like the doctor check.
func EvaluateControlPlaneBackupStale(source ControlPlaneBackupSource, r Rule, defaultMaxAge time.Duration, now time.Time) (Rule, string, error) {
	maxAge := r.ForDuration
	if maxAge <= 0 {
		maxAge = defaultMaxAge
	}
	if maxAge <= 0 {
		maxAge = DefaultControlPlaneBackupMaxAge
	}

	newest, ok, err := source.Newest()
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read newest control plane backup: %w", r.ID, err)
	}

	next := r
	next.LastEvaluatedAt = &now
	if !ok {
		next.PendingSince, next.Firing, next.FiringSince = nil, false, nil
		return next, "", nil
	}

	age := now.Sub(newest)
	v := age.Hours()
	next.LastValue = &v
	firing := age > maxAge

	var notice string
	if firing {
		notice = fmt.Sprintf("control plane: newest snapshot is %s old (limit %s)", age.Round(time.Minute), maxAge)
	}
	return advanceState(next, r, firing, 0, now), notice, nil
}
