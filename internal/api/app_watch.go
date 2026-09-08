package api

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"reflect"
	"time"

	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

// defaultWatchPollInterval is how often streamConditions re-reads stored
// reconcile conditions looking for a change, when Router.watchPollInterval
// is unset. Overridable via WithWatchPollInterval, the same "real value in
// prod, injectable in tests" shape defaultSessionTTL/WithSessionTTL already
// use.
const defaultWatchPollInterval = 2 * time.Second

// watchEvent is the SSE payload streamConditions emits each time the
// polled conditions change.
type watchEvent struct {
	Conditions []reconcile.Condition `json:"conditions"`
	ObservedAt time.Time             `json:"observed_at"`
}

func (rt *Router) watchPollIntervalOrDefault() time.Duration {
	if rt.watchPollInterval > 0 {
		return rt.watchPollInterval
	}
	return defaultWatchPollInterval
}

// handleAppWatch handles GET /api/v1/apps/{name}/watch: an SSE stream of
// the application controller's stored reconcile conditions, the live
// counterpart to handleDeployHistory's on-demand GET. It never touches
// reconciler internals: streamConditions below only polls the same
// GetConditions read handleDeployHistory already calls, on a timer.
func (rt *Router) handleAppWatch(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	_, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: app watch: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.streamConditions(w, r, applicationControllerName(name))
}

// handleDatabaseWatch is handleAppWatch's database analogue: GET
// /api/v1/databases/{name}/watch.
func (rt *Router) handleDatabaseWatch(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	_, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: database watch: load database failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	rt.streamConditions(w, r, databaseControllerName(name))
}

// streamConditions starts SSE on w, then polls
// rt.deploys.GetConditions(controllerName) every watchPollIntervalOrDefault,
// writing a watchEvent only when the result differs from the last one
// sent (reflect.DeepEqual, not performance-sensitive here). It stops
// when r.Context() is done, mirroring serveLiveDeployLog's own
// disconnect handling.
func (rt *Router) streamConditions(w http.ResponseWriter, r *http.Request, controllerName string) {
	flusher, ok := startSSE(w)
	if !ok {
		rt.logger.Error("api: watch: response writer does not support flushing", slog.String("controller", controllerName))
		return
	}

	// Same zero-body-flush gap deploy_attempts.go's own handlers fix:
	// write one byte so the browser confirms the connection is open.
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	ticker := time.NewTicker(rt.watchPollIntervalOrDefault())
	defer ticker.Stop()

	var last []reconcile.Condition
	sent := false

	for {
		conditions, err := rt.deploys.GetConditions(r.Context(), controllerName)
		if err != nil {
			rt.logger.Error("api: watch: get conditions failed", slog.String("error", err.Error()), slog.String("controller", controllerName))
		} else if !sent || !reflect.DeepEqual(last, conditions) {
			writeSSEEvent(w, watchEvent{Conditions: conditions, ObservedAt: time.Now()})
			flusher.Flush()
			last = conditions
			sent = true
		}

		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}
