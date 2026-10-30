package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/quota"
)

// GetQuota returns the explicit limits for a scope, or all zero (unlimited)
// when none were set.
func (db *DB) GetQuota(ctx context.Context, scope, scopeID string) (quota.Amounts, error) {
	var a quota.Amounts
	err := db.QueryRowContext(ctx, `
		SELECT max_apps, max_databases, max_memory_bytes, max_nano_cpus, max_previews
		FROM resource_quotas WHERE scope = ? AND scope_id = ?`, scope, scopeID).
		Scan(&a.Apps, &a.Databases, &a.MemoryBytes, &a.NanoCPUs, &a.Previews)
	if errors.Is(err, sql.ErrNoRows) {
		return quota.Amounts{}, nil
	}
	if err != nil {
		return quota.Amounts{}, fmt.Errorf("store: get quota %s/%s: %w", scope, scopeID, err)
	}
	return a, nil
}

// SetQuota replaces the limits for a scope.
func (db *DB) SetQuota(ctx context.Context, scope, scopeID string, a quota.Amounts) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO resource_quotas (scope, scope_id, max_apps, max_databases, max_memory_bytes, max_nano_cpus, max_previews, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(scope, scope_id) DO UPDATE SET
			max_apps = excluded.max_apps, max_databases = excluded.max_databases,
			max_memory_bytes = excluded.max_memory_bytes, max_nano_cpus = excluded.max_nano_cpus,
			max_previews = excluded.max_previews, updated_at = excluded.updated_at`,
		scope, scopeID, a.Apps, a.Databases, a.MemoryBytes, a.NanoCPUs, a.Previews, formatTime(time.Now()))
	if err != nil {
		return fmt.Errorf("store: set quota %s/%s: %w", scope, scopeID, err)
	}
	return nil
}

// DeleteQuota removes a scope's explicit limits.
func (db *DB) DeleteQuota(ctx context.Context, scope, scopeID string) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM resource_quotas WHERE scope = ? AND scope_id = ?`, scope, scopeID); err != nil {
		return fmt.Errorf("store: delete quota %s/%s: %w", scope, scopeID, err)
	}
	return nil
}

// QuotaScopeIDs returns the ids of every scope of one kind that has an
// explicit row, so the attention sweep only visits scopes that can be capped.
func (db *DB) QuotaScopeIDs(ctx context.Context, scope string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT scope_id FROM resource_quotas WHERE scope = ?`, scope)
	if err != nil {
		return nil, fmt.Errorf("store: list quota scopes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("store: scan quota scope: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// A service reserves its memory and CPU once per replica. /dev/shm is tmpfs
// charged to the memory cgroup, so it counts only when no memory limit bounds it.
const (
	quotaMemExpr = `(CASE WHEN COALESCE(json_extract(s.resources, '$.memory_bytes'), 0) > 0
		THEN json_extract(s.resources, '$.memory_bytes')
		ELSE COALESCE(json_extract(s.resources, '$.shm_size_bytes'), 0) END) * MAX(s.replicas, 1)`
	quotaCPUExpr = `COALESCE(json_extract(s.resources, '$.nano_cpus'), 0) * MAX(s.replicas, 1)`
)

// QuotaUsage totals what a team or project currently holds. Preview apps
// count toward memory and CPU but not toward the app count.
func (db *DB) QuotaUsage(ctx context.Context, scope, scopeID string) (quota.Amounts, error) {
	svcWhere, dbWhere := "s.project_id = ?", "d.project_id = ?"
	svcJoin, dbJoin := "", ""
	if scope == quota.ScopeTeam {
		svcJoin = "LEFT JOIN projects p ON p.id = s.project_id"
		dbJoin = "LEFT JOIN projects p ON p.id = d.project_id"
		svcWhere = "COALESCE(p.org_id, '" + DefaultTeamID + "') = ?"
		dbWhere = svcWhere
	}
	var u quota.Amounts
	err := db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN s.name NOT IN (SELECT preview_app_id FROM preview_environments) THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(`+quotaMemExpr+`), 0),
			COALESCE(SUM(`+quotaCPUExpr+`), 0)
		FROM desired_services s `+svcJoin+` WHERE `+svcWhere, scopeID).Scan(&u.Apps, &u.MemoryBytes, &u.NanoCPUs)
	if err != nil {
		return quota.Amounts{}, fmt.Errorf("store: quota usage apps %s/%s: %w", scope, scopeID, err)
	}
	var dbCount, dbMem, dbCPU int64
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*),
			COALESCE(SUM(COALESCE(json_extract(d.resources, '$.memory_bytes'), 0)), 0),
			COALESCE(SUM(COALESCE(json_extract(d.resources, '$.nano_cpus'), 0)), 0)
		FROM desired_databases d `+dbJoin+` WHERE `+dbWhere, scopeID).Scan(&dbCount, &dbMem, &dbCPU)
	if err != nil {
		return quota.Amounts{}, fmt.Errorf("store: quota usage databases %s/%s: %w", scope, scopeID, err)
	}
	u.Databases, u.MemoryBytes, u.NanoCPUs = dbCount, u.MemoryBytes+dbMem, u.NanoCPUs+dbCPU
	err = db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM preview_environments pe
		JOIN desired_services s ON s.name = pe.app_name `+svcJoin+`
		WHERE pe.status IN ('`+PreviewStatusDeploying+`', '`+PreviewStatusActive+`') AND `+svcWhere, scopeID).Scan(&u.Previews)
	if err != nil {
		return quota.Amounts{}, fmt.Errorf("store: quota usage previews %s/%s: %w", scope, scopeID, err)
	}
	return u, nil
}
