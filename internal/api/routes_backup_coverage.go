package api

import "net/http"

// registerBackupCoverageRoutes wires the coverage view, one-click protect-all
// and orphaned data. All are instance wide, so they stay closed to members of
// non-default teams.
func (rt *Router) registerBackupCoverageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/backups/coverage", rt.requireAbility(AbilityRead, rt.handleBackupCoverage))
	mux.HandleFunc("POST /api/v1/backups/protect-all", rt.requireAbility(AbilityWriteSensitive, rt.handleProtectAll))
	mux.HandleFunc("POST /api/v1/backups/encryption-key/ack", rt.requireAbility(AbilityWriteSensitive, rt.handleAckBackupEncryptionKey))
	mux.HandleFunc("GET /api/v1/storage/orphans", rt.requireAbility(AbilityRead, rt.handleListStorageOrphans))
	mux.HandleFunc("DELETE /api/v1/storage/orphans/{id}", rt.requireAbility(AbilityWriteSensitive, rt.handleReclaimStorageOrphan))
}
