package alerting

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// BackupCopyPending is one backup that has not reached the second destination yet.
type BackupCopyPending struct {
	Resource   string
	BackupID   string
	FinishedAt time.Time
	LastError  string
}

// BackupCopySource lists the backups still waiting for their second copy.
type BackupCopySource interface {
	PendingBackupCopies(ctx context.Context) ([]BackupCopyPending, error)
}

// EvaluateBackupCopyBehind runs one KindBackupCopyBehind rule. A backup is behind
// when its copy is still missing past the rule's ForDuration, else defaultLag.
func EvaluateBackupCopyBehind(ctx context.Context, source BackupCopySource, defaultLag time.Duration, r Rule, now time.Time) (Rule, string, error) {
	pending, err := source.PendingBackupCopies(ctx)
	if err != nil {
		return r, "", fmt.Errorf("alerting: evaluate rule %q: read pending backup copies: %w", r.ID, err)
	}
	limit := r.ForDuration
	if limit <= 0 {
		limit = defaultLag
	}
	var bad []string
	for _, p := range pending {
		age := now.Sub(p.FinishedAt)
		if age <= limit {
			continue
		}
		line := fmt.Sprintf("%s backup %s: no copy for %s", p.Resource, p.BackupID, age.Round(time.Minute))
		if p.LastError != "" {
			line += " (" + truncateNotice(p.LastError, 120) + ")"
		}
		bad = append(bad, line)
	}
	next := r
	next.LastEvaluatedAt = &now
	v := float64(len(bad))
	next.LastValue = &v
	var notice string
	if len(bad) > 0 {
		notice = "backup copy behind: " + strings.Join(bad, "; ")
	}
	return advanceState(next, r, len(bad) > 0, 0, now), notice, nil
}
