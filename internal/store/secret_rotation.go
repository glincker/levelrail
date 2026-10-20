package store

import (
	"context"
	"fmt"
	"time"
)

// secretRotationCutoffLayout matches the fixed-width, 3-decimal-fraction
// timestamp shape every updated_at column this file compares against was
// written in (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), used by
// SaveSecretValue and Set{Project,Organization,Environment}SecretEnvVar
// alike): CountStaleSecrets formats its cutoff with this same layout so
// the WHERE clause's string comparison stays lexicographically correct,
// the same reasoning FormatAuditTime's own doc comment gives for
// audit_log.created_at.
const secretRotationCutoffLayout = "2006-01-02T15:04:05.000Z"

// CountStaleSecrets returns how many secret-backed values, across every
// tier that carries one (per-app service_secret_values, plus the
// secret-marked rows of the three shared-env tiers: project_env_vars,
// organization_env_vars, environment_env_vars), were last set or rotated
// strictly before cutoff. Used by GET /api/v1/system/doctor's
// stale_secrets check (internal/api/doctor.go) to surface one aggregate
// "N secrets need rotation" count instead of an operator having to check
// every app and every shared-env scope individually.
func (db *DB) CountStaleSecrets(ctx context.Context, cutoff time.Time) (int, error) {
	cutoffStr := cutoff.UTC().Format(secretRotationCutoffLayout)
	var n int
	err := db.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM service_secret_values WHERE updated_at < ?) +
			(SELECT COUNT(*) FROM project_env_vars WHERE is_secret = 1 AND updated_at < ?) +
			(SELECT COUNT(*) FROM organization_env_vars WHERE is_secret = 1 AND updated_at < ?) +
			(SELECT COUNT(*) FROM environment_env_vars WHERE is_secret = 1 AND updated_at < ?)
	`, cutoffStr, cutoffStr, cutoffStr, cutoffStr).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("store: count stale secrets: %w", err)
	}
	return n, nil
}
