package archive

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	maxRowsPerBundle = 50000
	maxRunsPerBundle = 5000
	deleteBatch      = 400
	rollbackKeep     = 2
)

type target struct {
	table string
	ids   []string
}

func scanRows(rows *sql.Rows) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("archive: columns: %w", err)
	}
	var out []map[string]any
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("archive: scan: %w", err)
		}
		row := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				vals[i] = string(b)
			}
			row[c] = vals[i]
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("archive: rows: %w", err)
	}
	return out, nil
}

func query(ctx context.Context, db *sql.DB, q string, args ...any) ([]map[string]any, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("archive: query: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanRows(rows)
}

func idOf(row map[string]any) string { return fmt.Sprint(row["id"]) }

// collect writes every cold row of kind to w and returns what to delete once
// the bundle is durable. Deploy attempts keep the newest successful ones per
// service (rollback targets) and any attempt a probe row still points at.
func collect(ctx context.Context, db *sql.DB, kind Kind, cutoff time.Time, w *bundleWriter) ([]target, error) {
	switch kind {
	case KindAudit:
		rows, err := query(ctx, db, `SELECT * FROM audit_log WHERE created_at < ? ORDER BY created_at LIMIT ?`, store.FormatAuditTime(cutoff), maxRowsPerBundle)
		if err != nil {
			return nil, err
		}
		return writeSimple(w, "audit_log", rows)
	case KindDeploys:
		rows, err := query(ctx, db, `SELECT d.* FROM deploy_attempts d
			WHERE d.finished_at IS NOT NULL AND d.finished_at <> '' AND d.started_at < ?
			AND NOT EXISTS (SELECT 1 FROM probe_attempts p WHERE p.deploy_attempt_id = d.id)
			AND d.id NOT IN (SELECT id FROM (SELECT id, ROW_NUMBER() OVER (PARTITION BY service_name ORDER BY started_at DESC) AS rn
				FROM deploy_attempts WHERE status = ?) WHERE rn <= ?)
			ORDER BY d.started_at LIMIT ?`, cutoff.UTC().Format(time.RFC3339Nano), store.DeployAttemptStatusSucceeded, rollbackKeep, maxRowsPerBundle)
		if err != nil {
			return nil, err
		}
		return writeSimple(w, "deploy_attempts", rows)
	case KindPipelines:
		return collectPipelines(ctx, db, cutoff, w)
	}
	return nil, fmt.Errorf("archive: unknown kind %q", kind)
}

func writeSimple(w *bundleWriter, table string, rows []map[string]any) ([]target, error) {
	t := target{table: table}
	for _, r := range rows {
		if err := w.add(table, r); err != nil {
			return nil, err
		}
		t.ids = append(t.ids, idOf(r))
	}
	return []target{t}, nil
}

func collectPipelines(ctx context.Context, db *sql.DB, cutoff time.Time, w *bundleWriter) ([]target, error) {
	runs, err := query(ctx, db, `SELECT * FROM pipeline_runs WHERE status IN (?, ?, ?) AND created_at < ? ORDER BY created_at LIMIT ?`,
		store.PipelineStatusSucceeded, store.PipelineStatusFailed, store.PipelineStatusCancelled, cutoff.UTC().Format(time.RFC3339Nano), maxRunsPerBundle)
	if err != nil {
		return nil, err
	}
	t := target{table: "pipeline_runs"}
	for _, r := range runs {
		if err := w.add("pipeline_runs", r); err != nil {
			return nil, err
		}
		jobs, err := query(ctx, db, `SELECT * FROM pipeline_jobs WHERE run_id = ?`, idOf(r))
		if err != nil {
			return nil, err
		}
		for _, j := range jobs {
			if err := w.add("pipeline_jobs", j); err != nil {
				return nil, err
			}
			steps, err := query(ctx, db, `SELECT * FROM pipeline_steps WHERE job_id = ?`, idOf(j))
			if err != nil {
				return nil, err
			}
			for _, s := range steps {
				if err := w.add("pipeline_steps", s); err != nil {
					return nil, err
				}
			}
		}
		t.ids = append(t.ids, idOf(r))
	}
	return []target{t}, nil
}

func deleteTargets(ctx context.Context, db *sql.DB, targets []target) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("archive: begin delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, t := range targets {
		for i := 0; i < len(t.ids); i += deleteBatch {
			end := min(i+deleteBatch, len(t.ids))
			batch := t.ids[i:end]
			ph := make([]byte, 0, len(batch)*2)
			args := make([]any, len(batch))
			for j, id := range batch {
				if j > 0 {
					ph = append(ph, ',')
				}
				ph = append(ph, '?')
				args[j] = id
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM "+t.table+" WHERE id IN ("+string(ph)+")", args...); err != nil { //nolint:gosec // table is from a fixed set, ids are bound
				return fmt.Errorf("archive: delete from %s: %w", t.table, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("archive: commit delete: %w", err)
	}
	return nil
}
