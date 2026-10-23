package api

import "net/http"

// registerPublicRoutes registers the brand, demo, system, onboarding, updates and changelog routes.
func (rt *Router) registerPublicRoutes(mux *http.ServeMux) {
	// Public: the frontend needs branding before a session exists (e.g.
	// on the login screen itself).
	mux.HandleFunc("GET /api/v1/brand", rt.handleBrand)
	mux.HandleFunc("GET /api/v1/dev-mode", rt.handleDevMode)
	mux.HandleFunc("GET /api/v1/demo", rt.handleDemoInfo)
	mux.HandleFunc("POST /api/v1/demo/session", rt.handleDemoSession)
	mux.HandleFunc("POST /api/v1/demo/cta", rt.handleDemoCTA)
	// Route metadata for the in-app API explorer (web/src/routes/settings/api-explorer.tsx):
	// AbilityRead, same tier as system/status, since this is the API
	// surface's own shape, not resource data.
	mux.HandleFunc("GET /api/v1/openapi.json", rt.requireAbility(AbilityRead, rt.handleOpenAPISpec))

	// System status (General settings page): configured/not-configured
	// signals plus disk usage, AbilityRead like everything else an
	// authenticated operator can passively view.
	mux.HandleFunc("GET /api/v1/system/status", rt.requireAbility(AbilityRead, rt.handleSystemStatus))
	// Doctor (levelrail-cli doctor): a superset preflight bundle of the
	// same individual checks above, plus a few doctor-only ones (data
	// dir writability, ingress port availability, SQLite reachability),
	// AbilityRead like system/status above.
	mux.HandleFunc("GET /api/v1/system/doctor", rt.requireAbility(AbilityRead, rt.handleSystemDoctor))
	// Same checks as the report: list them, stream results as each finishes, re-run one. Fixes apply through POST /api/v1/fixes/{id}/apply (root, audit-logged).
	mux.HandleFunc("GET /api/v1/system/resilience", rt.requireAbility(AbilityRead, rt.handleGetResilience))
	mux.HandleFunc("POST /api/v1/system/resilience/simulate", rt.requireAbility(AbilityWrite, rt.handleSimulateResilience))
	mux.HandleFunc("POST /api/v1/system/resilience/snapshot", rt.requireAbility(AbilityRoot, rt.handleResilienceFix))
	mux.HandleFunc("GET /api/v1/system/doctor/checks", rt.requireAbility(AbilityRead, rt.handleListDoctorChecks))
	mux.HandleFunc("GET /api/v1/system/doctor/stream", rt.requireAbility(AbilityRead, rt.withStreamReauth(doctorStreamResource, rt.handleStreamDoctor)))
	mux.HandleFunc("POST /api/v1/system/doctor/checks/{id}/run", rt.requireAbility(AbilityRead, rt.handleRunDoctorCheck))
	mux.HandleFunc("GET /api/v1/system/reverse-proxy", rt.requireAbility(AbilityRead, rt.handleReverseProxyGuide))
	// Every container on this node, Levelrail-managed or not (containers.go's
	// own doc comment on why a Managed one stays read-only here),
	// AbilityRead like system/status above.
	mux.HandleFunc("GET /api/v1/system/containers", rt.requireAbility(AbilityRead, rt.handleListContainers))
	// Orphaned containers only (containers_orphaned.go's
	// requireOrphanedContainer re-confirms this server-side on every
	// call, never trusting the client). Stop/remove are raw docker-level
	// mutations fleet-wide, not scoped to one app's own desired state:
	// AbilityRoot, the same tier system/prune and orphaned volume
	// cleanup already sit behind. Claim creates an ordinary app through
	// the same path POST /api/v1/apps itself uses, so it stays
	// AbilityWrite, that route's own tier.
	mux.HandleFunc("POST /api/v1/system/containers/{name}/stop", rt.requireAbility(AbilityRoot, rt.handleStopOrphanedContainer))
	mux.HandleFunc("POST /api/v1/system/containers/{name}/remove", rt.requireAbility(AbilityRoot, rt.handleRemoveOrphanedContainer))
	mux.HandleFunc("POST /api/v1/system/containers/{name}/claim", rt.requireAbility(AbilityWrite, rt.handleClaimOrphanedContainer))
	// POST /system/prune deletes real Docker resources (stopped
	// containers, dangling images, anonymous volumes, unused build
	// cache) fleet-wide, not scoped to one app: AbilityRoot, the same
	// gate handleDrainNode uses for its own fleet-wide, no-undo action,
	// not AbilityWrite (which a narrower, single-app token could hold).
	mux.HandleFunc("POST /api/v1/system/prune", rt.requireAbility(AbilityRoot, rt.handleSystemPrune))
	// Control plane self-backups hold every stored secret ciphertext and
	// token hash, so every verb is AbilityRoot, including the read ones.
	mux.HandleFunc("POST /api/v1/system/backups", rt.requireAbility(AbilityRoot, rt.handleCreateControlPlaneBackup))
	mux.HandleFunc("GET /api/v1/system/backups", rt.requireAbility(AbilityRoot, rt.handleListControlPlaneBackups))
	mux.HandleFunc("GET /api/v1/system/backups/{name}/download", rt.requireAbility(AbilityRoot, rt.handleDownloadControlPlaneBackup))
	mux.HandleFunc("POST /api/v1/system/backups/{name}/verify", rt.requireAbility(AbilityRoot, rt.handleVerifyControlPlaneBackup))
	mux.HandleFunc("DELETE /api/v1/system/backups/{name}", rt.requireAbility(AbilityRoot, rt.handleDeleteControlPlaneBackup))
	// Off-box encrypted control plane backups, restore drills and escrow
	// (control_plane_dr.go). Status reads are AbilityRead and carry public
	// recipients only. Changing the destination or recipients is
	// AbilityWriteSensitive; building an escrow bundle touches the master
	// key, so it is AbilityRoot. Restore replaces the live database so it runs offline; restore-plan gives the steps and exact command.
	mux.HandleFunc("GET /api/v1/system/control-plane-dr", rt.requireAbility(AbilityRead, rt.handleGetControlPlaneDR))
	mux.HandleFunc("PUT /api/v1/system/control-plane-dr/settings", rt.requireAbility(AbilityWriteSensitive, rt.handleUpdateControlPlaneDR))
	mux.HandleFunc("GET /api/v1/system/control-plane-dr/backups", rt.requireAbility(AbilityRead, rt.handleListControlPlaneDRBackups))
	mux.HandleFunc("POST /api/v1/system/control-plane-dr/run", rt.requireAbility(AbilityWriteSensitive, rt.handleRunControlPlaneDRBackup))
	mux.HandleFunc("POST /api/v1/system/control-plane-dr/drill", rt.requireAbility(AbilityWriteSensitive, rt.handleRunControlPlaneDRDrill))
	mux.HandleFunc("GET /api/v1/system/control-plane-dr/restore-plan", rt.requireAbility(AbilityRead, rt.handleGetControlPlaneRestorePlan))
	mux.HandleFunc("POST /api/v1/system/control-plane-dr/keys", rt.requireAbility(AbilityWriteSensitive, rt.handleGenerateControlPlaneDRKeypair))
	mux.HandleFunc("POST /api/v1/system/control-plane-dr/escrow", rt.requireAbility(AbilityRoot, rt.handleControlPlaneDREscrow))
	mux.HandleFunc("POST /api/v1/system/control-plane-dr/escrow/ack", rt.requireAbility(AbilityWriteSensitive, rt.handleAckControlPlaneDREscrow))
	// Orphaned named volumes: detection is a read (AbilityRead), the
	// cleanup that actually deletes one is the same AbilityRoot,
	// fleet-wide, no-undo tier system/prune sits behind, not
	// AbilityWrite.
	mux.HandleFunc("GET /api/v1/system/orphans", rt.requireAbility(AbilityRead, rt.handleListOrphans))
	mux.HandleFunc("POST /api/v1/system/orphans/reap", rt.requireAbility(AbilityRoot, rt.handleReapOrphans))
	mux.HandleFunc("GET /api/v1/system/volumes/orphaned", rt.requireAbility(AbilityRead, rt.handleListOrphanedVolumes))
	mux.HandleFunc("POST /api/v1/system/volumes/orphaned/cleanup", rt.requireAbility(AbilityRoot, rt.handleCleanupOrphanedVolumes))
	// Master key rotation re-wraps every stored DEK live: AbilityRoot,
	// the same fleet-wide-blast-radius tier as prune above, not
	// AbilityWrite (SecretSetter's own gate for a single app's values).
	mux.HandleFunc("POST /api/v1/system/master-key/rotate", rt.requireAbility(AbilityRoot, rt.handleRotateMasterKey))
	mux.HandleFunc("GET /api/v1/system/secrets/binding", rt.requireAbility(AbilityRead, rt.handleGetSecretBinding))
	mux.HandleFunc("POST /api/v1/system/secrets/rebind", rt.requireAbility(AbilityRoot, rt.handleRebindSecrets))

	// Setup wizard state. Completing stays AbilityWrite so creating a first
	// app any other way can dismiss it; step progress is admin-only.
	mux.HandleFunc("GET /api/v1/onboarding", rt.requireAbility(AbilityRead, rt.handleGetOnboardingState))
	mux.HandleFunc("POST /api/v1/onboarding/complete", rt.requireAbility(AbilityWrite, rt.handleCompleteOnboarding))
	mux.HandleFunc("PUT /api/v1/onboarding/progress", rt.requireAbility(AbilityRoot, rt.handleUpdateOnboardingProgress))

	// Updates (Settings > Updates page): running version vs. GitHub's
	// latest published release, AbilityRead like system/status above.
	mux.HandleFunc("GET /api/v1/updates", rt.requireAbility(AbilityRead, rt.handleGetUpdates))
	mux.HandleFunc("GET /api/v1/updates/preflight", rt.requireAbility(AbilityRead, rt.handleUpdatePreflight))
	mux.HandleFunc("GET /api/v1/updates/releases", rt.requireAbility(AbilityRead, rt.handleReleaseHistory))
	mux.HandleFunc("GET /api/v1/updates/rollback-plan", rt.requireAbility(AbilityRoot, rt.handleRollbackPlan))
	mux.HandleFunc("GET /api/v1/updates/history", rt.requireAbility(AbilityRead, rt.handleUpgradeHistory))
	mux.HandleFunc("POST /api/v1/updates/history/{id}/ack", rt.requireAbility(AbilityWrite, rt.handleAckUpgrade))
	mux.HandleFunc("GET /api/v1/system/readiness", rt.requireAbility(AbilityRead, rt.handleServerReadiness))
	mux.HandleFunc("GET /api/v1/updates/self-upgrade/plan", rt.requireAbility(AbilityRead, rt.handleSelfUpgradePlan))
	mux.HandleFunc("GET /api/v1/updates/self-upgrade/attempts", rt.requireAbility(AbilityRead, rt.handleSelfUpgradeAttempts))
	mux.HandleFunc("GET /api/v1/updates/self-upgrade/status", rt.requireAbility(AbilityRead, rt.handleSelfUpgradeStatus))
	mux.HandleFunc("POST /api/v1/updates/self-upgrade", rt.requireAbility(AbilityRoot, rt.handleSelfUpgradeStart))
	mux.HandleFunc("GET /api/v1/updates/restore-points", rt.requireAbility(AbilityRoot, rt.handleRestorePointList))
	mux.HandleFunc("POST /api/v1/updates/restore-points/{id}/restore", rt.requireAbility(AbilityRoot, rt.handleRestorePointRestore))
	mux.HandleFunc("POST /api/v1/updates/self-upgrade/attempts/{id}/fix", rt.requireAbility(AbilityWrite, rt.handleSelfUpgradeAttemptFix))
	mux.HandleFunc("POST /api/v1/updates/self-upgrade/attempts/{id}/retry", rt.requireAbility(AbilityWrite, rt.handleSelfUpgradeAttemptRetry))
	mux.HandleFunc("POST /api/v1/updates/self-upgrade/attempts/{id}/rollback", rt.requireAbility(AbilityWrite, rt.handleSelfUpgradeAttemptRollback))
	mux.HandleFunc("POST /api/v1/updates/rollback", rt.requireAbility(AbilityRoot, rt.handleRollbackStart))
	mux.HandleFunc("GET /api/v1/updates/schedule", rt.requireAbility(AbilityRead, rt.handleGetUpgradeSchedule))
	mux.HandleFunc("POST /api/v1/updates/schedule", rt.requireAbility(AbilityRoot, rt.handleCreateUpgradeSchedule))
	mux.HandleFunc("DELETE /api/v1/updates/schedule", rt.requireAbility(AbilityRoot, rt.handleCancelUpgradeSchedule))
	mux.HandleFunc("GET /api/v1/updates/auto", rt.requireAbility(AbilityRoot, rt.handleGetUpgradePolicy))
	mux.HandleFunc("PUT /api/v1/updates/auto", rt.requireAbility(AbilityRoot, rt.handleUpdateUpgradePolicy))
	mux.HandleFunc("POST /api/v1/updates/backup-now", rt.requireAbility(AbilityRoot, rt.handleUpgradeBackupNow))
	// Channel/auto-update settings are AbilityRoot on both verbs: see
	// handleGetUpdateSettings' own doc comment (updates_settings.go).
	mux.HandleFunc("GET /api/v1/updates/settings", rt.requireAbility(AbilityRoot, rt.handleGetUpdateSettings))
	mux.HandleFunc("PUT /api/v1/updates/settings", rt.requireAbility(AbilityRoot, rt.handleUpdateSettings))

	// "What's new" dashboard panel: recent entries parsed from the
	// repo's own CHANGELOG.md, AbilityRead like updates above.
	mux.HandleFunc("GET /api/v1/changelog", rt.requireAbility(AbilityRead, rt.handleGetChangelog))
}
