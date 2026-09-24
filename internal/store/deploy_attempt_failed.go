package store

import (
	"context"
	"fmt"
	"time"
)

// FailedDeploy is an app whose most recent deploy attempt failed.
// LastGoodImage is the image of its newest succeeded attempt, empty if none.
type FailedDeploy struct {
	Attempt       DeployAttempt
	LastGoodImage string
}

// ListFailedDeploysSince returns, per existing app, its latest attempt when
// that attempt failed and started at or after since, newest first.
func (db *DB) ListFailedDeploysSince(ctx context.Context, since time.Time) ([]FailedDeploy, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT d.id, d.service_name, d.image, d.commit_sha, d.source, d.status, d.started_at, d.finished_at, d.error, d.config_snapshot, d.detected_framework,
			COALESCE((SELECT g.image FROM deploy_attempts g
				WHERE g.service_name = d.service_name AND g.status = ? AND g.image <> ''
				ORDER BY g.started_at DESC LIMIT 1), '')
		FROM deploy_attempts d
		WHERE d.status = ?
			AND d.service_name IN (SELECT name FROM desired_services)
			AND d.started_at = (SELECT MAX(l.started_at) FROM deploy_attempts l WHERE l.service_name = d.service_name)
		ORDER BY d.started_at DESC
	`, DeployAttemptStatusSucceeded, DeployAttemptStatusFailed)
	if err != nil {
		return nil, fmt.Errorf("store: list failed deploys: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []FailedDeploy
	for rows.Next() {
		var lastGood string
		a, err := scanDeployAttempt(func(dest ...any) error {
			return rows.Scan(append(dest, &lastGood)...)
		})
		if err != nil {
			return nil, fmt.Errorf("store: scan failed deploy row: %w", err)
		}
		if a.StartedAt.Before(since) {
			continue
		}
		out = append(out, FailedDeploy{Attempt: *a, LastGoodImage: lastGood})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate failed deploy rows: %w", err)
	}
	return out, nil
}
