package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/database"
	"github.com/GLINCKER/levelrail/internal/store"
)

// engineStatsTimeout bounds the live stats call: the page polls it.
const engineStatsTimeout = 10 * time.Second

// engineStatObject is one index or collection and its document or point count.
type engineStatObject struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// databaseEngineStatsResource is GET /api/v1/databases/{name}/engine-stats:
// live counts for engines with no data browser, read from the engine's own
// API inside its container.
type databaseEngineStatsResource struct {
	Engine  string             `json:"engine"`
	Objects []engineStatObject `json:"objects"`
	Total   int64              `json:"total"`
	// Metrics holds Memcached's numeric server stats, by stat name.
	Metrics map[string]int64 `json:"metrics,omitempty"`
}

// memcachedStatKeys are the server stats the dashboard shows.
var memcachedStatKeys = []string{"curr_items", "bytes", "limit_maxbytes", "get_hits", "get_misses", "evictions", "curr_connections", "uptime"}

// handleGetDatabaseEngineStats handles GET /api/v1/databases/{name}/engine-stats.
func (rt *Router) handleGetDatabaseEngineStats(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	desired, err := rt.databases.GetDesiredDatabase(r.Context(), name)
	if errors.Is(err, store.ErrDatabaseNotFound) {
		writeError(w, http.StatusNotFound, "database not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: engine stats: load database failed", err, slog.String("name", name))
		return
	}
	if !store.IsSearchEngine(desired.Engine) && desired.Engine != store.EngineMemcached {
		writeError(w, http.StatusBadRequest, "engine stats are not available for engine "+desired.Engine)
		return
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "exec is not configured on this control plane")
		return
	}
	nodeRuntime, state, ok := rt.resolveDatabaseExecContainer(w, r, name, desired.NodeID)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), engineStatsTimeout)
	defer cancel()
	if desired.Engine == store.EngineMemcached {
		rt.writeMemcachedStats(ctx, w, nodeRuntime, state.ID, name)
		return
	}
	content, err := backup.ReadSearchContent(ctx, nodeRuntime, desired.Engine, state.ID)
	if err != nil {
		rt.logger.Warn("api: engine stats: read failed", slog.String("name", name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "the engine did not answer its stats API")
		return
	}
	out := databaseEngineStatsResource{Engine: desired.Engine, Objects: make([]engineStatObject, 0, len(content))}
	for n, c := range content {
		out.Objects = append(out.Objects, engineStatObject{Name: n, Count: c})
	}
	sort.Slice(out.Objects, func(i, j int) bool { return out.Objects[i].Name < out.Objects[j].Name })
	out.Total = content.Total()
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) writeMemcachedStats(ctx context.Context, w http.ResponseWriter, nodeRuntime docker.Runtime, containerID, name string) {
	rc, err := nodeRuntime.Exec(ctx, containerID, database.MemcachedStatsCmd())
	var stats map[string]string
	if err == nil {
		raw, readErr := io.ReadAll(io.LimitReader(rc, 1<<16))
		_ = rc.Close()
		err = readErr
		if err == nil {
			stats, err = database.ParseMemcachedStats(string(raw))
		}
	}
	if err != nil {
		rt.logger.Warn("api: engine stats: memcached stats failed", slog.String("name", name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "the engine did not answer its stats command")
		return
	}
	out := databaseEngineStatsResource{Engine: store.EngineMemcached, Objects: []engineStatObject{}, Metrics: map[string]int64{}}
	for _, k := range memcachedStatKeys {
		if n, perr := strconv.ParseInt(stats[k], 10, 64); perr == nil {
			out.Metrics[k] = n
		}
	}
	out.Total = out.Metrics["curr_items"]
	writeJSON(w, http.StatusOK, out)
}
