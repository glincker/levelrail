package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// resourceIDForDatabase is telemetry's stable identifier for one managed
// database's metrics/logs, the database-kind counterpart to
// resourceIDForApp (metrics.go). Must match what cmd/levelrail/main.go's
// telemetryTargets/logTargets write samples and log entries under, the
// same rename hazard resourceIDForApp's own doc comment calls out.
func resourceIDForDatabase(name string) string {
	return "database:" + name
}

// lookupDatabaseResource is the database-kind resourceLookup, backing
// handleQueryDatabaseLogs, handleLiveDatabaseLogStream, and
// handleQueryDatabaseMetrics (database_metrics.go).
func (rt *Router) lookupDatabaseResource(ctx context.Context, name string) (string, bool, error) {
	if _, err := rt.databases.GetDesiredDatabase(ctx, name); errors.Is(err, store.ErrDatabaseNotFound) {
		return "", false, nil
	} else if err != nil {
		return "", false, err
	}
	return resourceIDForDatabase(name), true, nil
}

// handleQueryDatabaseLogs handles GET /api/v1/databases/{name}/logs: the
// database-kind counterpart to handleQueryLogs (logs.go); both share
// queryResourceLogs's implementation, differing only in the
// resourceLookup and the operation/noun strings used in log lines and
// the 404 message.
func (rt *Router) handleQueryDatabaseLogs(w http.ResponseWriter, r *http.Request) {
	rt.queryResourceLogs(w, r, rt.lookupDatabaseResource, "query database logs", "database")
}

// handleLiveDatabaseLogStream handles
// GET /api/v1/databases/{name}/logs/stream: the database-kind
// counterpart to handleLiveLogStream (live_logs.go); both share
// streamResourceLogs's implementation, differing only in the
// resourceLookup and the operation/noun strings used in log lines and
// the 404 message.
func (rt *Router) handleLiveDatabaseLogStream(w http.ResponseWriter, r *http.Request) {
	rt.streamResourceLogs(w, r, rt.lookupDatabaseResource, "live database log stream", "database")
}
