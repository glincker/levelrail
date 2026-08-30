package api

import "net/http"

// handleQueryDatabaseMetrics handles GET /api/v1/databases/{name}/metrics:
// the database-kind counterpart to handleQueryMetrics (metrics.go); both
// share queryResourceMetrics's implementation, differing only in the
// resourceLookup and the operation/noun strings used in log lines and
// the 404 message.
func (rt *Router) handleQueryDatabaseMetrics(w http.ResponseWriter, r *http.Request) {
	rt.queryResourceMetrics(w, r, rt.lookupDatabaseResource, "query database metrics", "database")
}
