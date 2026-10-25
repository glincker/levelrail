package store

import (
	"context"
	"time"
)

// ResolvePendingDeployAttempts settles the attempts queued for an offline node
// once the node is back and the app has reconciled: the newest becomes
// succeeded (the desired state it recorded is what just applied) and older
// ones superseded. Returns how many rows moved.
func (db *DB) ResolvePendingDeployAttempts(ctx context.Context, serviceName string, now time.Time) (int, error) {
	pending, err := db.queryDeployAttempts(ctx, "pending",
		`service_name = ? AND status = ? ORDER BY started_at DESC, id DESC`, serviceName, DeployAttemptStatusPending)
	if err != nil {
		return 0, err
	}
	moved := 0
	for i, a := range pending {
		var ok bool
		if i == 0 {
			ok, err = db.finishPending(ctx, a.ID, DeployAttemptStatusSucceeded, "", "", now)
		} else {
			ok, err = db.finishPending(ctx, a.ID, DeployAttemptStatusSuperseded, DeployReasonSuperseded, pending[0].ID, now)
		}
		if err != nil {
			return moved, err
		}
		if ok {
			moved++
		}
	}
	return moved, nil
}

func (db *DB) finishPending(ctx context.Context, id, status, reason, by string, now time.Time) (bool, error) {
	res, err := db.ExecContext(ctx, `
		UPDATE deploy_attempts SET status = ?, reason = ?, superseded_by = ?, finished_at = ?
		WHERE id = ? AND status = ?
	`, status, reason, by, now.UTC().Format(time.RFC3339Nano), id, DeployAttemptStatusPending)
	return movedRows(res, err, "resolve pending deploy attempt", id)
}
