package store

import (
	"context"
	"fmt"
	"time"
)

// Hook types HookRun.HookType holds, matching
// internal/reconcile/application's own unexported constants of the same
// values (that package can't import this one's vocabulary any more than
// store.DesiredService's own doc comment lets it import spec's, so the
// two are kept in sync by value, not by a shared import).
const (
	HookTypePreDeploy  = "pre_deploy"
	HookTypePostDeploy = "post_deploy"
)

// HookRun is one outcome of running a service's pre/post-deploy hook
// command (internal/spec.Hooks, store.ServiceHooks), persisted by
// UpsertHookRun so GET /api/v1/apps/{name}/hook-runs can show what
// actually happened without a live log viewer having been open at the
// time.
type HookRun struct {
	ServiceName string
	HookType    string
	Command     string
	ExitCode    int
	Success     bool
	Output      string
	RanAt       time.Time
}

// UpsertHookRun replaces the stored outcome for (run.ServiceName,
// run.HookType): only the most recent run per hook type is kept, see
// migrations/0083_service_hook_runs.sql's own comment for why a full
// history isn't.
func (db *DB) UpsertHookRun(ctx context.Context, run HookRun) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO service_hook_runs (service_name, hook_type, command, exit_code, success, output, ran_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (service_name, hook_type) DO UPDATE SET
			command = excluded.command,
			exit_code = excluded.exit_code,
			success = excluded.success,
			output = excluded.output,
			ran_at = excluded.ran_at
	`, run.ServiceName, run.HookType, run.Command, run.ExitCode, run.Success, run.Output, run.RanAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("store: upsert hook run %s/%s: %w", run.ServiceName, run.HookType, err)
	}
	return nil
}

// GetHookRuns returns every stored hook outcome for serviceName (at most
// one per hook type), or nil if neither hook has ever run.
func (db *DB) GetHookRuns(ctx context.Context, serviceName string) ([]HookRun, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT service_name, hook_type, command, exit_code, success, output, ran_at
		FROM service_hook_runs
		WHERE service_name = ?
		ORDER BY hook_type
	`, serviceName)
	if err != nil {
		return nil, fmt.Errorf("store: get hook runs for %q: %w", serviceName, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var out []HookRun
	for rows.Next() {
		var run HookRun
		var ranAt string
		if err := rows.Scan(&run.ServiceName, &run.HookType, &run.Command, &run.ExitCode, &run.Success, &run.Output, &ranAt); err != nil {
			return nil, fmt.Errorf("store: scan hook run row: %w", err)
		}
		t, err := time.Parse(time.RFC3339Nano, ranAt)
		if err != nil {
			return nil, fmt.Errorf("store: parse ran_at %q: %w", ranAt, err)
		}
		run.RanAt = t
		out = append(out, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store: iterate hook run rows: %w", err)
	}
	return out, nil
}
