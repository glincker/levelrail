package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/reconcile/application"
)

// handleListStorageEnvKeys handles GET /api/v1/storage-env-keys: every
// env var name application.StorageEnvKeys can inject into a container
// when an app has a storage target attached. Frontend consumers (the
// storage-attachment card's pre-attach collision warning) read this
// instead of hardcoding the list as TS literals, matching
// handleListDatabaseEngines' own precedent (database_engines.go) for
// a static, backend-owned registry exposed to avoid a second,
// independently-maintained copy in TypeScript.
func (rt *Router) handleListStorageEnvKeys(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, application.StorageEnvKeys)
}
