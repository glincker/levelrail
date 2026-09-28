package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// PipelineRunRow is one run in the cross-app overview. It carries no logs,
// steps, or definition snapshot.
type PipelineRunRow struct {
	ID                string
	PipelineID        string
	PipelineName      string
	AppName           string
	Number            int
	TriggerKind       string
	Ref               string
	CommitSHA         string
	Status            string
	Reason            string
	CreatedAt         time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	HoldState         string
	PendingApprovalID int64
	ApprovalAbility   string
}

// PipelineRunFilter narrows ListPipelineRunRows. Status accepts a run status
// or the virtual value "held" (a run awaiting a hold decision); "running"
// matches queued and running runs.
type PipelineRunFilter struct {
	Status   string
	App      string
	Pipeline string
	Trigger  string
	// BeforeCreated and BeforeID form the keyset cursor: only rows strictly
	// older than this position are returned.
	BeforeCreated time.Time
	BeforeID      string
	Limit         int
}

const pipelineRunRowCols = `r.id, r.pipeline_id, COALESCE(p.name, ''), r.app_name, r.number, r.trigger_kind, r.ref, r.commit_sha,
	r.status, r.reason, r.created_at, r.started_at, r.finished_at, r.hold_state,
	COALESCE((SELECT a.id FROM pipeline_approvals a WHERE a.run_id = r.id AND a.decision = '' ORDER BY a.id LIMIT 1), 0),
	COALESCE((SELECT a.required_ability FROM pipeline_approvals a WHERE a.run_id = r.id AND a.decision = '' ORDER BY a.id LIMIT 1), '')`

// ListPipelineRunRows returns runs across all apps, newest first.
func (db *DB) ListPipelineRunRows(ctx context.Context, f PipelineRunFilter) ([]PipelineRunRow, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	var conds []string
	var args []any
	switch f.Status {
	case "":
	case "held":
		conds = append(conds, `r.hold_state = 'pending'`)
	case PipelineStatusRunning:
		conds = append(conds, `r.status IN ('queued', 'running')`)
	default:
		conds = append(conds, `r.status = ?`)
		args = append(args, f.Status)
	}
	for _, c := range []struct{ col, val string }{{"r.app_name", f.App}, {"p.name", f.Pipeline}, {"r.trigger_kind", f.Trigger}} {
		if c.val != "" {
			conds = append(conds, c.col+` = ?`)
			args = append(args, c.val)
		}
	}
	if !f.BeforeCreated.IsZero() {
		ts := formatTime(f.BeforeCreated)
		conds = append(conds, `(r.created_at < ? OR (r.created_at = ? AND r.id < ?))`)
		args = append(args, ts, ts, f.BeforeID)
	}
	q := `SELECT ` + pipelineRunRowCols + ` FROM pipeline_runs r LEFT JOIN pipelines p ON p.id = r.pipeline_id`
	if len(conds) > 0 {
		q += ` WHERE ` + strings.Join(conds, ` AND `)
	}
	q += ` ORDER BY r.created_at DESC, r.id DESC LIMIT ?`
	rows, err := db.QueryContext(ctx, q, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("store: list pipeline run rows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineRunRow
	for rows.Next() {
		var r PipelineRunRow
		var created string
		var started, finished sql.NullString
		if err := rows.Scan(&r.ID, &r.PipelineID, &r.PipelineName, &r.AppName, &r.Number, &r.TriggerKind, &r.Ref, &r.CommitSHA,
			&r.Status, &r.Reason, &created, &started, &finished, &r.HoldState, &r.PendingApprovalID, &r.ApprovalAbility); err != nil {
			return nil, fmt.Errorf("store: scan pipeline run row: %w", err)
		}
		if r.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
			return nil, fmt.Errorf("store: parse pipeline run created_at: %w", err)
		}
		if r.StartedAt, err = parseTimePtr(started); err != nil {
			return nil, fmt.Errorf("store: parse pipeline run started_at: %w", err)
		}
		if r.FinishedAt, err = parseTimePtr(finished); err != nil {
			return nil, fmt.Errorf("store: parse pipeline run finished_at: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// PipelineAppCounts is one app's run tally for the overview summary.
type PipelineAppCounts struct {
	AppName         string
	Running         int
	Succeeded24h    int
	Failed24h       int
	Cancelled24h    int
	WaitingApproval int
	Held            int
}

// PipelineAppCounts tallies runs per app: active runs and pending decisions
// regardless of age, finished runs since the given time.
func (db *DB) PipelineAppCounts(ctx context.Context, since time.Time) ([]PipelineAppCounts, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT app_name,
			SUM(CASE WHEN status IN ('queued', 'running') THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'waiting_approval' THEN 1 ELSE 0 END),
			SUM(CASE WHEN hold_state = 'pending' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'succeeded' AND created_at >= ?1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'failed' AND created_at >= ?1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'cancelled' AND created_at >= ?1 THEN 1 ELSE 0 END)
		FROM pipeline_runs GROUP BY app_name`, formatTime(since))
	if err != nil {
		return nil, fmt.Errorf("store: pipeline app counts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []PipelineAppCounts
	for rows.Next() {
		var c PipelineAppCounts
		if err := rows.Scan(&c.AppName, &c.Running, &c.WaitingApproval, &c.Held, &c.Succeeded24h, &c.Failed24h, &c.Cancelled24h); err != nil {
			return nil, fmt.Errorf("store: scan pipeline app counts: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
