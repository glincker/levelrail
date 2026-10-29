package api

import "net/http"

// registerBackupRoutes registers the databases, backups, restore and usage routes.
func (rt *Router) registerBackupRoutes(mux *http.ServeMux) {
	// Backup history and manual trigger, per database. Trigger needs
	// AbilityWriteSensitive: it starts real work against a live bucket
	// using a previously-stored credential, the same sensitivity class
	// creating or deleting the backup target itself already carries.
	// History listing is ordinary AbilityRead.
	mux.HandleFunc("POST /api/v1/databases/{name}/backups", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleTriggerBackup))
	mux.HandleFunc("GET /api/v1/databases/{name}/backups", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleListBackupHistory))

	// Instance-wide backup history across every database and app volume,
	// the aggregated counterpart of the per-resource routes above and
	// below: ordinary AbilityRead, same tier as those, since this is only
	// ever a merged read of history metadata already visible per-resource.
	mux.HandleFunc("GET /api/v1/backups", rt.requireAbility(AbilityRead, rt.handleListAllBackups))
	mux.HandleFunc("GET /api/v1/usage/summary", rt.requireAbility(AbilityRead, rt.handleUsageSummary))

	// Backup download streams the dump bytes: AbilityReadSensitive, since it is a read but exposes a
	// full database. Not AbilityRoot: unlike restore it destroys nothing.
	mux.HandleFunc("GET /api/v1/databases/{name}/backups/{historyId}/download", rt.requireAbilityForResource(AbilityReadSensitive, databaseResourceFromPath, rt.handleDownloadBackup))

	// Delete one specific archived backup on demand, rather than waiting
	// for retention (BackupRetain/BackupRetainDays above) to age it out.
	// AbilityWriteSensitive, matching the manual trigger route above: this
	// destroys a real stored artifact, the same sensitivity class as
	// triggering a backup or deleting the backup target itself, not the
	// AbilityRoot tier restore below uses, since nothing here touches a
	// live database's own data. See handleDeleteBackup's own doc comment.
	mux.HandleFunc("DELETE /api/v1/databases/{name}/backups/{historyId}", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleDeleteBackup))

	// Backup verification: re-download a succeeded backup and confirm it
	// is still intact (checksum, size, and a lightweight structural
	// check), without ever attempting a live restore. AbilityWriteSensitive
	// for the trigger, matching the manual backup trigger route above
	// exactly (see handleVerifyBackup's own doc comment for why); listing
	// past attempts is ordinary AbilityRead, matching history listing
	// above.
	mux.HandleFunc("POST /api/v1/databases/{name}/backups/{historyId}/verify", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleVerifyBackup))
	mux.HandleFunc("GET /api/v1/databases/{name}/backups/{historyId}/verifications", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleListBackupVerifications))

	// Scheduled backup config: AbilityWriteSensitive, it decides where unattended backups go.
	// No GET: handleGetDatabase already returns these fields.
	mux.HandleFunc("PUT /api/v1/databases/{name}/backup-schedule", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleSetBackupSchedule))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/backup-schedule", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleClearBackupSchedule))

	// Public host access per database: AbilityWriteSensitive, it changes network exposure.
	// No GET: handleGetDatabase already returns the fields.
	mux.HandleFunc("PUT /api/v1/databases/{name}/public-access", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleSetDatabasePublicAccess))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/public-access", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleClearDatabasePublicAccess))

	// Stop/start, per database (database_stop_start.go). AbilityWriteSensitive,
	// same tier public-access above uses: taking a database offline (and,
	// on start, back online) is real operational impact, not an ordinary
	// desired-state edit.
	mux.HandleFunc("POST /api/v1/databases/{name}/stop", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleStopDatabase))
	mux.HandleFunc("POST /api/v1/databases/{name}/start", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleStartDatabase))

	// Restore, per database (restore.go). AbilityRoot, not
	// AbilityWriteSensitive: see handleTriggerRestore's own doc comment
	// for why this, alone among every backup-related route, needs the
	// same top tier as node management and placement. History listing is
	// ordinary AbilityRead, the same boundary the backup history route
	// above already draws.
	mux.HandleFunc("POST /api/v1/databases/{name}/restore", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleTriggerRestore))
	mux.HandleFunc("GET /api/v1/databases/{name}/restores", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleListRestoreHistory))

	// Point-in-time restore (pitr.go): enabling/disabling PITR is
	// AbilityWriteSensitive, the same tier creating a backup target or
	// triggering an ordinary backup already uses; base backups are the
	// physical counterpart of an ordinary backup, same tier again.
	// Triggering an actual PITR restore is AbilityRoot, matching the
	// ordinary restore route above for the identical reason: it
	// overwrites a live database's actual data with no way back.
	mux.HandleFunc("POST /api/v1/databases/{name}/pitr", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleEnablePITR))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/pitr", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleDisablePITR))
	mux.HandleFunc("GET /api/v1/databases/{name}/pitr", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleGetPITRStatus))
	mux.HandleFunc("POST /api/v1/databases/{name}/base-backups", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleTriggerBaseBackup))
	mux.HandleFunc("GET /api/v1/databases/{name}/base-backups", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleListBaseBackupHistory))
	mux.HandleFunc("POST /api/v1/databases/{name}/pitr-restore", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleTriggerPITRRestore))
	mux.HandleFunc("GET /api/v1/databases/{name}/pitr-restores", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleListPITRRestoreHistory))
	mux.HandleFunc("POST /api/v1/databases/{name}/major-upgrade", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleMajorUpgrade))
	mux.HandleFunc("GET /api/v1/databases/{name}/major-upgrades", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleListMajorUpgrades))
	mux.HandleFunc("POST /api/v1/databases/{name}/major-upgrades/{id}/rollback", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleRollbackMajorUpgrade))
	mux.HandleFunc("DELETE /api/v1/databases/{name}/major-upgrades/{id}/snapshot", rt.requireAbilityForResource(AbilityRoot, databaseResourceFromPath, rt.handleDiscardMajorUpgradeSnapshot))

	// App service volume backups (app_volume_backups.go/
	// app_volume_backup_download.go/app_volume_backup_verify.go): the
	// exact same ability tiers as the database routes just above, applied
	// to a service's named volume instead of a managed database. See
	// those handlers' own doc comments for the per-route reasoning this
	// mirrors.
	mux.HandleFunc("GET /api/v1/backups/health", rt.requireAbility(AbilityRead, rt.handleBackupHealth))
	mux.HandleFunc("GET /api/v1/backups/drills", rt.requireAbility(AbilityRead, rt.handleListBackupDrills))
	mux.HandleFunc("GET /api/v1/backups/drills/{id}", rt.requireAbility(AbilityRead, rt.handleGetBackupDrill))
	mux.HandleFunc("POST /api/v1/backups/drills", rt.requireAbility(AbilityWriteSensitive, rt.handleStartBackupDrill))
	mux.HandleFunc("POST /api/v1/backups/protection/refresh", rt.requireAbility(AbilityWriteSensitive, rt.handleRefreshBackupProtection))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/backup-policy", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetVolumeBackupPolicy))
	mux.HandleFunc("PUT /api/v1/apps/{name}/volumes/{volume}/backup-policy", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetVolumeBackupPolicy))
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/restore-to", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleVolumeRestoreTo))
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/backups", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleTriggerVolumeBackup))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/backups", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListVolumeBackupHistory))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/download", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleDownloadVolumeBackup))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleDeleteVolumeBackup))
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verify", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleVerifyVolumeBackup))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/backups/{historyId}/verifications", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListVolumeBackupVerifications))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/backup-schedule", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetVolumeBackupSchedule))
	mux.HandleFunc("PUT /api/v1/apps/{name}/volumes/{volume}/backup-schedule", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetVolumeBackupSchedule))

	// Volume file browser: reads need read:sensitive (contents can hold secrets), edits need write, so an operator can fix a config file without SSH.
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/files", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleListVolumeFiles))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/files/content", rt.requireAbilityForResource(AbilityReadSensitive, appResourceFromPath, rt.handleDownloadVolumeFile))
	mux.HandleFunc("PUT /api/v1/apps/{name}/volumes/{volume}/files/content", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handlePutVolumeFile))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/volumes/{volume}/files/content", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteVolumeFile))
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/files/restore", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleRestoreVolumeFile))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/volumes/{volume}/backup-schedule", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleClearVolumeBackupSchedule))
	// AbilityRoot, matching the database restore route above exactly: see
	// handleTriggerVolumeRestore's own doc comment for why this is not a
	// lesser risk tier just because the target is a filesystem.
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/restore", rt.requireAbilityForResource(AbilityRoot, appResourceFromPath, rt.handleTriggerVolumeRestore))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/restores", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListVolumeRestoreHistory))
	// Restore as a new volume (app_volume_clone_restore.go): the
	// non-destructive counterpart just above, the same AbilityWriteSensitive
	// tier the database clone-restore route below uses, see
	// handleVolumeCloneRestore's own doc comment for why.
	mux.HandleFunc("POST /api/v1/apps/{name}/volumes/{volume}/restore-as-new", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleVolumeCloneRestore))
	mux.HandleFunc("GET /api/v1/apps/{name}/volumes/{volume}/clone-restores", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListVolumeCloneRestores))
	// Restore as a new database (database_clone_restore.go): the
	// non-destructive counterpart just above, AbilityWriteSensitive
	// rather than AbilityRoot, see handleCloneRestore's own doc comment
	// for why this endpoint doesn't need the top tier the in-place
	// restore route does. History listing is ordinary AbilityRead, the
	// same boundary the in-place restore history route above draws.
	mux.HandleFunc("POST /api/v1/databases/{name}/restore-as-new", rt.requireAbilityForResource(AbilityWriteSensitive, databaseResourceFromPath, rt.handleCloneRestore))
	mux.HandleFunc("GET /api/v1/databases/{name}/clone-restores", rt.requireAbilityForResource(AbilityRead, databaseResourceFromPath, rt.handleListCloneRestores))

	// Object-storage attachment per app: AbilityWriteSensitive, it decides which bucket credentials
	// the container receives. No GET: handleGetApp already returns storage_target_id.
	mux.HandleFunc("PUT /api/v1/apps/{name}/storage", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppStorage))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/storage", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleClearAppStorage))

	// Real app-to-database attachment (apps_database.go): AbilityWrite,
	// not AbilityDeploy, since this is a config write, not itself a
	// deploy trigger (the next reconcile picks up the change on its own
	// schedule, the same "desired state changes, containers converge
	// later" shape PUT .../storage/.../node/.../project already have).
	mux.HandleFunc("PUT /api/v1/apps/{name}/database", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppDatabase))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/database", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleClearAppDatabase))
	// Multi-connection app-to-database (apps_connections.go): the
	// DatabaseEnv-map-backed sibling of PUT/DELETE .../database just
	// above, for an app that connects to more than one managed database.
	// Same AbilityWrite tier and "config write, not a deploy trigger"
	// reasoning; GET is AbilityRead like any other app-detail read.
	mux.HandleFunc("GET /api/v1/apps/{name}/connections", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppConnections))
	mux.HandleFunc("POST /api/v1/apps/{name}/connections", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleCreateAppConnection))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/connections/{env_var}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteAppConnection))
	mux.HandleFunc("GET /api/v1/apps/{name}/connectable-databases", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListConnectableDatabases))
	// One Vault-sourced env var declaration at a time (apps_vault_env.go):
	// same AbilityWrite tier and "config write, not a deploy trigger"
	// reasoning as PUT/DELETE .../database just above.
	mux.HandleFunc("PUT /api/v1/apps/{name}/vault-env/{key}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppVaultEnv))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/vault-env/{key}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleClearAppVaultEnv))
	// One preview-specific env var override at a time (apps_preview_env.go):
	// same AbilityWrite tier and "config write, not a deploy trigger"
	// reasoning as PUT/DELETE .../vault-env just above; only takes effect
	// the next time a preview environment is created from this app.
	mux.HandleFunc("PUT /api/v1/apps/{name}/preview-env/{key}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppPreviewEnvOverride))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/preview-env/{key}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleClearAppPreviewEnvOverride))
	// Branch-scoped env var overrides (apps_branch_env.go): a narrower,
	// pattern-matched sibling of PUT/DELETE .../preview-env just above,
	// only applied when a preview's own branch matches. POST can carry a
	// secret-marked value, so it's gated at AbilityWriteSensitive, the
	// same tier PUT .../secrets/{key} uses; GET and DELETE never expose
	// or require a value, so they stay at AbilityRead/AbilityWrite.
	mux.HandleFunc("GET /api/v1/apps/{name}/branch-env", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppBranchEnv))
	mux.HandleFunc("POST /api/v1/apps/{name}/branch-env", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppBranchEnv))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/branch-env/{id}", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteAppBranchEnv))
	mux.HandleFunc("GET /api/v1/apps/{name}/contexts", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleListAppContexts))
	mux.HandleFunc("PUT /api/v1/apps/{name}/contexts", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppContext))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/contexts", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleDeleteAppContext))
	mux.HandleFunc("GET /api/v1/apps/{name}/effective-config", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetEffectiveConfig))
	// Outbound allowlist per app: AbilityWriteSensitive, it changes the exfiltration surface.
	// GET has its own route since an unset policy is the common case.
	mux.HandleFunc("GET /api/v1/apps/{name}/egress-policy", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppEgressPolicy))
	mux.HandleFunc("PUT /api/v1/apps/{name}/egress-policy", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleSetAppEgressPolicy))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/egress-policy", rt.requireAbilityForResource(AbilityWriteSensitive, appResourceFromPath, rt.handleClearAppEgressPolicy))
	mux.HandleFunc("GET /api/v1/apps/{name}/health", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleGetAppHealth))
	mux.HandleFunc("PUT /api/v1/apps/{name}/health", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppHealth))
	mux.HandleFunc("DELETE /api/v1/apps/{name}/health", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleClearAppHealth))
	// Health check auto-detect (apps_health_discover.go): AbilityRead, the
	// same tier GET .../diagnose already uses for an active probe against
	// a running container (routes.go), since this changes nothing and
	// only reads what the container's own HTTP port already exposes.
	mux.HandleFunc("POST /api/v1/apps/{name}/health/discover", rt.requireAbilityForResource(AbilityRead, appResourceFromPath, rt.handleDiscoverAppHealth))
	// Attach/detach a named Docker volume outside a redeploy
	// (apps_volumes_attach.go): AbilityWrite, the same tier health above
	// uses, since this is an ordinary declarative resource, not a secret
	// or network-exfiltration surface like egress-policy/storage.
	mux.HandleFunc("PUT /api/v1/apps/{name}/volumes", rt.requireAbilityForResource(AbilityWrite, appResourceFromPath, rt.handleSetAppVolumes))
	rt.registerAppFilesRoutes(mux)
}
