package api

import (
	"net/http"

	"github.com/GLINCKER/levelrail/internal/deploy"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// authorizeDatabaseRefs checks the caller may see each named database and
// answers a refusal like an unknown database. It writes the response itself.
func (rt *Router) authorizeDatabaseRefs(w http.ResponseWriter, r *http.Request, names ...string) bool {
	denied, err := rt.firstDeniedRef(r, AbilityRead, databaseRefs(names))
	if err != nil {
		rt.internalError(w, "api: database ref authz", err)
		return false
	}
	if denied != nil {
		writeError(w, http.StatusBadRequest, "unknown database")
		return false
	}
	return true
}

// authorizeSpecDatabaseRefs applies authorizeDatabaseRefs to every database a
// spec's { from: ... } env vars reference.
func (rt *Router) authorizeSpecDatabaseRefs(w http.ResponseWriter, r *http.Request, services map[string]spec.Service) bool {
	return rt.authorizeDatabaseRefs(w, r, deploy.DatabaseRefs(services)...)
}
