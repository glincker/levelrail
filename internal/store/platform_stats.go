package store

import (
	"context"
	"fmt"
	"time"
)

// PlatformStats is a point-in-time read of control plane health numbers
// that metric exporters publish as self metrics.
type PlatformStats struct {
	// ReconcileLagSeconds is the age of the stalest controller status row.
	ReconcileLagSeconds  float64
	ReconcileControllers int
	DeploysQueued        int
	DeploysRunning       int
	// DeployLastSeconds and DeployAvgSeconds cover deploys finished within
	// the window passed to ReadPlatformStats.
	DeployLastSeconds float64
	DeployAvgSeconds  float64
	DeploysSucceeded  int
	DeploysFailed     int
	NodesByStatus     map[NodeStatus]int
}

// ReadPlatformStats gathers PlatformStats with a few aggregate queries.
func (db *DB) ReadPlatformStats(ctx context.Context, now time.Time, window time.Duration) (PlatformStats, error) {
	out := PlatformStats{NodesByStatus: map[NodeStatus]int{}}

	var oldest *string
	if err := db.QueryRowContext(ctx, `SELECT MIN(newest), COUNT(*) FROM (
		SELECT MAX(updated_at) AS newest FROM reconcile_status GROUP BY controller_name)`).
		Scan(&oldest, &out.ReconcileControllers); err != nil {
		return out, fmt.Errorf("store: read reconcile ages: %w", err)
	}
	if oldest != nil {
		if t, err := time.Parse(time.RFC3339Nano, *oldest); err == nil {
			out.ReconcileLagSeconds = max(0, now.Sub(t).Seconds())
		}
	}

	if err := db.readActiveDeploys(ctx, &out); err != nil {
		return out, err
	}
	if err := db.readFinishedDeploys(ctx, now, window, &out); err != nil {
		return out, err
	}

	nodes, err := db.QueryContext(ctx, `SELECT status, COUNT(*) FROM nodes GROUP BY status`)
	if err != nil {
		return out, fmt.Errorf("store: count nodes: %w", err)
	}
	defer func() { _ = nodes.Close() }()
	for nodes.Next() {
		var status string
		var n int
		if err := nodes.Scan(&status, &n); err != nil {
			return out, fmt.Errorf("store: scan node counts: %w", err)
		}
		out.NodesByStatus[NodeStatus(status)] = n
	}
	return out, nodes.Err()
}

func (db *DB) readActiveDeploys(ctx context.Context, out *PlatformStats) error {
	rows, err := db.QueryContext(ctx, `SELECT status, COUNT(*) FROM deploy_attempts WHERE status IN (?, ?) GROUP BY status`,
		DeployAttemptStatusQueued, DeployAttemptStatusRunning)
	if err != nil {
		return fmt.Errorf("store: count active deploys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var status string
		var n int
		if err := rows.Scan(&status, &n); err != nil {
			return fmt.Errorf("store: scan active deploys: %w", err)
		}
		if status == DeployAttemptStatusQueued {
			out.DeploysQueued = n
		} else {
			out.DeploysRunning = n
		}
	}
	return rows.Err()
}

func (db *DB) readFinishedDeploys(ctx context.Context, now time.Time, window time.Duration, out *PlatformStats) error {
	since := now.Add(-window).UTC().Format(time.RFC3339Nano)
	rows, err := db.QueryContext(ctx, `SELECT status, started_at, finished_at FROM deploy_attempts
		WHERE finished_at IS NOT NULL AND finished_at >= ? AND status IN (?, ?)
		ORDER BY finished_at DESC LIMIT 1000`, since, DeployAttemptStatusSucceeded, DeployAttemptStatusFailed)
	if err != nil {
		return fmt.Errorf("store: list finished deploys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var total float64
	var timed int
	for rows.Next() {
		var status, started, finished string
		if err := rows.Scan(&status, &started, &finished); err != nil {
			return fmt.Errorf("store: scan finished deploy: %w", err)
		}
		if status == DeployAttemptStatusSucceeded {
			out.DeploysSucceeded++
		} else {
			out.DeploysFailed++
		}
		s, errS := time.Parse(time.RFC3339Nano, started)
		f, errF := time.Parse(time.RFC3339Nano, finished)
		if errS != nil || errF != nil || f.Before(s) {
			continue
		}
		d := f.Sub(s).Seconds()
		if timed == 0 {
			out.DeployLastSeconds = d
		}
		total += d
		timed++
	}
	if timed > 0 {
		out.DeployAvgSeconds = total / float64(timed)
	}
	return rows.Err()
}
