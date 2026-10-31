package api

import "net/http"

// registerMailBackupRoutes wires mail backup, restore and the backup
// destination (ADR 060 section D). Running a backup is root only because
// it prunes older restore points. A real restore also needs step-up.
func (rt *Router) registerMailBackupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/mail/backups", rt.requireAbility(AbilityRead, rt.handleListMailBackups))
	mux.HandleFunc("POST /api/v1/mail/backups", rt.requireAbility(AbilityRoot, rt.handleRunMailBackup))
	mux.HandleFunc("POST /api/v1/mail/backups/{id}/restore", rt.requireAbility(AbilityRoot, rt.handleRestoreMailBackup))
	mux.HandleFunc("GET /api/v1/mail/storage", rt.requireAbility(AbilityRead, rt.handleGetMailStorage))
	mux.HandleFunc("PUT /api/v1/mail/storage", rt.requireAbility(AbilityRoot, rt.handlePutMailStorage))
	mux.HandleFunc("POST /api/v1/mail/storage/test", rt.requireAbility(AbilityRoot, rt.handleTestMailStorage))
}
