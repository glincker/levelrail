package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// UpdateServiceNode reassigns svc to run on nodeID ("" for this control
// plane's own local node, per the placement migration's comment), the
// only way node_id ever changes: SaveDesiredService's own doc comment
// explains why it's deliberately excluded from that method's
// full-record-replace semantics.
func (db *DB) UpdateServiceNode(ctx context.Context, name, nodeID string) error {
	if err := db.updateServiceRow(ctx, "update node", name, `
		UPDATE desired_services SET node_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, nodeID, name); err != nil {
		return err
	}
	return db.noteAppChange(ctx, name)
}

// UpdateServiceProject reassigns svc to projectID, the only writer of
// project_id (see SaveDesiredService). Empty is stored as SQL NULL because
// the column is nullable, unlike node_id.
func (db *DB) UpdateServiceProject(ctx context.Context, name, projectID string) error {
	if err := db.updateServiceRow(ctx, "update project", name, `
		UPDATE desired_services SET project_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, sql.NullString{String: projectID, Valid: projectID != ""}, name); err != nil {
		return err
	}
	return db.noteAppChange(ctx, name)
}

// UpdateServiceStorageTarget reassigns svc's storage credential source, the
// only writer of storage_target_id. Empty is stored as SQL NULL. The caller
// validates the target exists; this method just writes.
func (db *DB) UpdateServiceStorageTarget(ctx context.Context, name, storageTargetID string) error {
	return db.updateServiceRow(ctx, "update storage target", name, `
		UPDATE desired_services SET storage_target_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, sql.NullString{String: storageTargetID, Valid: storageTargetID != ""}, name)
}

// UpdateServiceEgressPolicy replaces svc's egress allowlist as a whole
// (policy nil clears it), without SaveDesiredService's full-record replace.
func (db *DB) UpdateServiceEgressPolicy(ctx context.Context, name string, policy *ServiceEgressPolicy) error {
	return db.updateServiceJSONColumn(ctx, name, "egress policy", `
		UPDATE desired_services SET egress_policy = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, policy)
}

// UpdateServicePlacementRules replaces an app's tolerations and affinity as a
// whole, without SaveDesiredService's full-record replace.
func (db *DB) UpdateServicePlacementRules(ctx context.Context, name string, tj, aj json.RawMessage) error {
	return db.updateServiceRow(ctx, "update placement rules", name, `
		UPDATE desired_services SET tolerations = ?, affinity = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, string(tj), string(aj), name)
}

// UpdateServiceVolumes replaces svc's named-volume list as a whole,
// without SaveDesiredService's full-record replace: the dedicated
// dual-write path Volumes lacked until now, mirroring
// UpdateServiceEgressPolicy (app.yaml's volumes: block also sets this,
// via the ordinary SaveDesiredService replace). A nil slice persists as
// "[]", matching SaveDesiredService's own nil-to-empty handling, so a
// service with no volumes always reads back an empty slice, never nil.
func (db *DB) UpdateServiceVolumes(ctx context.Context, name string, volumes []ServiceVolume) error {
	if volumes == nil {
		volumes = []ServiceVolume{}
	}
	return db.updateServiceJSONColumn(ctx, name, "volumes", `
		UPDATE desired_services SET volumes = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, volumes)
}

// UpdateServiceHealth replaces svc's readiness/liveness config as a whole
// (health nil clears it), without SaveDesiredService's full-record replace.
func (db *DB) UpdateServiceHealth(ctx context.Context, name string, health *ServiceHealth) error {
	return db.updateServiceJSONColumn(ctx, name, "health", `
		UPDATE desired_services SET health = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, health)
}

func (db *DB) updateServiceJSONColumn(ctx context.Context, name, what, query string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("store: update %s for service %q: marshal: %w", what, name, err)
	}
	return db.updateServiceRow(ctx, "update "+what, name, query, string(encoded), name)
}

// updateServiceRow runs one UPDATE keyed by service name and maps zero
// affected rows to ErrServiceNotFound.
func (db *DB) updateServiceRow(ctx context.Context, what, name, query string, args ...any) error {
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("store: %s for service %q: %w", what, name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: %s for service %q: rows affected: %w", what, name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return nil
}

// UpdateServiceDatabaseAttachment is DesiredService.DatabaseAttachment's
// only writer, the same separation-from-ordinary-update reasoning
// UpdateServiceStorageTarget already establishes for StorageTargetID.
// att nil clears the attachment (all three columns back to ”).
func (db *DB) UpdateServiceDatabaseAttachment(ctx context.Context, name string, att *DatabaseAttachment) error {
	var dbName, envVar, field string
	if att != nil {
		dbName, envVar, field = att.DatabaseName, att.EnvVar, att.Field
	}
	return db.updateServiceRow(ctx, "update database attachment", name, `
		UPDATE desired_services
		SET database_attachment_name = ?, database_attachment_env_var = ?, database_attachment_field = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE name = ?
	`, dbName, envVar, field, name)
}

// SetServiceVaultEnvVar adds, replaces (ref non-nil) or removes (ref nil) one
// VaultEnv entry. The read-modify-write runs in one transaction so concurrent
// writers of other keys cannot lose each other's update.
func (db *DB) SetServiceVaultEnvVar(ctx context.Context, name, envVar string, ref *VaultEnvRef) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set vault env var for service %q: begin transaction: %w", name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	var vaultEnvJSON string
	err = tx.QueryRowContext(ctx, `SELECT vault_env FROM desired_services WHERE name = ?`, name).Scan(&vaultEnvJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServiceNotFound
	}
	if err != nil {
		return fmt.Errorf("store: set vault env var for service %q: read existing: %w", name, err)
	}

	vaultEnv := map[string]VaultEnvRef{}
	if vaultEnvJSON != "" {
		if err := json.Unmarshal([]byte(vaultEnvJSON), &vaultEnv); err != nil {
			return fmt.Errorf("store: set vault env var for service %q: decode existing: %w", name, err)
		}
	}
	if ref == nil {
		delete(vaultEnv, envVar)
	} else {
		vaultEnv[envVar] = *ref
	}

	updated, err := json.Marshal(vaultEnv)
	if err != nil {
		return fmt.Errorf("store: set vault env var for service %q: encode: %w", name, err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE desired_services SET vault_env = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, string(updated), name); err != nil {
		return fmt.Errorf("store: set vault env var for service %q: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set vault env var for service %q: commit: %w", name, err)
	}
	return nil
}

// SetServiceDatabaseEnvVar adds, replaces (ref non-nil) or removes (ref nil)
// one DatabaseEnv entry, in one transaction like SetServiceVaultEnvVar.
// DatabaseAttachment keeps its own setter.
func (db *DB) SetServiceDatabaseEnvVar(ctx context.Context, name, envVar string, ref *DatabaseEnvRef) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store: set database env var for service %q: begin transaction: %w", name, err)
	}
	defer func() {
		_ = tx.Rollback() // no-op if Commit already succeeded
	}()

	var databaseEnvJSON string
	err = tx.QueryRowContext(ctx, `SELECT database_env FROM desired_services WHERE name = ?`, name).Scan(&databaseEnvJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrServiceNotFound
	}
	if err != nil {
		return fmt.Errorf("store: set database env var for service %q: read existing: %w", name, err)
	}

	databaseEnv := map[string]DatabaseEnvRef{}
	if databaseEnvJSON != "" {
		if err := json.Unmarshal([]byte(databaseEnvJSON), &databaseEnv); err != nil {
			return fmt.Errorf("store: set database env var for service %q: decode existing: %w", name, err)
		}
	}
	if ref == nil {
		delete(databaseEnv, envVar)
	} else {
		databaseEnv[envVar] = *ref
	}

	updated, err := json.Marshal(databaseEnv)
	if err != nil {
		return fmt.Errorf("store: set database env var for service %q: encode: %w", name, err)
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE desired_services SET database_env = ?, env_dirty = 1, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, string(updated), name); err != nil {
		return fmt.Errorf("store: set database env var for service %q: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store: set database env var for service %q: commit: %w", name, err)
	}
	return nil
}

// UpdateServiceApp assigns svc to app appID, the only way
// desired_services.app_id ever changes outside migrations/0039_apps.sql's
// own backfill: SaveDesiredService's own doc comment explains why this
// is deliberately excluded from that method's full-record-replace
// semantics, the same reasoning UpdateServiceNode/UpdateServiceProject
// already establish for NodeID/ProjectID.
func (db *DB) UpdateServiceApp(ctx context.Context, name, appID string) error {
	return db.updateServiceRow(ctx, "update app", name, `
		UPDATE desired_services SET app_id = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, appID, name)
}

// UpdateServiceMaintenanceHold sets or clears (hold == "") the service's maintenance hold.
func (db *DB) UpdateServiceMaintenanceHold(ctx context.Context, name, hold string) error {
	res, err := db.ExecContext(ctx, `UPDATE desired_services SET maintenance_hold = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`, hold, name)
	if err != nil {
		return fmt.Errorf("store: update maintenance hold for service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: update maintenance hold for service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return fmt.Errorf("store: update maintenance hold for service %q: %w", name, ErrServiceNotFound)
	}
	return nil
}

// UpdateServiceSuspended is the only way suspended ever changes, the
// same "own single-purpose setter, excluded from SaveDesiredService"
// reasoning UpdateServiceNode/UpdateServiceProject/
// UpdateServiceStorageTarget already establish. Setting it true does
// not by itself stop any container: internal/reconcile/application's
// controller is what converges to zero containers once it observes
// Suspended on its next reconcile, the same level-triggered separation
// DeleteDesiredService's own doc comment describes for delete.
func (db *DB) UpdateServiceSuspended(ctx context.Context, name string, suspended bool) error {
	query := `UPDATE desired_services SET suspended = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`
	if !suspended {
		// Resuming clears env_dirty too: removeStale tore down every
		// replica while suspended, so the next reconcile creates brand
		// new containers from current desired state, the same "fresh
		// container creation" moment RestartService already clears it on.
		query = `UPDATE desired_services SET suspended = ?, env_dirty = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?`
	}
	if err := db.updateServiceRow(ctx, "update suspended", name, query, suspended, name); err != nil {
		return err
	}
	return db.noteAppChange(ctx, name)
}

// SetServiceAutoRollbackOnCrashloop is the only way
// auto_rollback_on_crashloop ever changes, the same "own single-purpose
// setter, excluded from SaveDesiredService" reasoning UpdateServiceNode/
// UpdateServiceProject/UpdateServiceStorageTarget/UpdateServiceSuspended
// already establish.
func (db *DB) SetServiceAutoRollbackOnCrashloop(ctx context.Context, name string, enabled bool) error {
	return db.updateServiceRow(ctx, "update auto rollback on crashloop", name, `
		UPDATE desired_services SET auto_rollback_on_crashloop = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, enabled, name)
}

// SetServiceAutoRollbackOnSLOBurn is the only way
// auto_rollback_on_slo_burn ever changes, the same "own single-purpose
// setter, excluded from SaveDesiredService" reasoning
// SetServiceAutoRollbackOnCrashloop already establishes. mode must be one
// of the AutoRollbackSLOBurn* constants; the caller (handleSetAutoRollbackSLOBurn)
// validates that before calling this.
func (db *DB) SetServiceAutoRollbackOnSLOBurn(ctx context.Context, name, mode string) error {
	return db.updateServiceRow(ctx, "update auto rollback on slo burn", name, `
		UPDATE desired_services SET auto_rollback_on_slo_burn = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, mode, name)
}

// SetServiceBadgeEnabled is the only way badge_enabled ever changes, the
// same "own single-purpose setter, excluded from SaveDesiredService"
// reasoning SetServiceExecEnabled already establishes.
func (db *DB) SetServiceBadgeEnabled(ctx context.Context, name string, enabled bool) error {
	return db.updateServiceRow(ctx, "update badge enabled", name, `
		UPDATE desired_services SET badge_enabled = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, enabled, name)
}

// SetServiceExecEnabled is the only way exec_enabled ever changes, the
// same "own single-purpose setter, excluded from SaveDesiredService"
// reasoning SetServiceAutoRollbackOnCrashloop and
// UpdateServiceNode/UpdateServiceProject/UpdateServiceStorageTarget/
// UpdateServiceSuspended already establish.
func (db *DB) SetServiceExecEnabled(ctx context.Context, name string, enabled bool) error {
	return db.updateServiceRow(ctx, "update exec enabled", name, `
		UPDATE desired_services SET exec_enabled = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, enabled, name)
}

// RestartService is the only writer of restart_nonce (see SaveDesiredService).
// The value is only compared for equality, so short random hex is enough.
func (db *DB) RestartService(ctx context.Context, name string) error {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Errorf("store: restart service %q: generate nonce: %w", name, err)
	}
	nonce := hex.EncodeToString(buf)

	// Also clears env_dirty: a restart recreates the container from
	// current desired state, the same "fresh container creation" moment
	// DesiredService.EnvDirty's own doc comment says clears it.
	res, err := db.ExecContext(ctx, `
		UPDATE desired_services SET restart_nonce = ?, env_dirty = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE name = ?
	`, nonce, name)
	if err != nil {
		return fmt.Errorf("store: restart service %q: %w", name, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store: restart service %q: rows affected: %w", name, err)
	}
	if n == 0 {
		return ErrServiceNotFound
	}
	return db.noteAppChange(ctx, name)
}
