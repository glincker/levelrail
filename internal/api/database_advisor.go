package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/glincker/levelrail/internal/dbadvisor"
	"github.com/glincker/levelrail/internal/dbviewer"
	"github.com/glincker/levelrail/internal/store"
)

// advisorMaxConcurrent bounds advisor runs per database so repeated calls cannot pile onto one server.
const advisorMaxConcurrent = 2

var advisorSlots = struct {
	mu sync.Mutex
	by map[string]chan struct{}
}{by: map[string]chan struct{}{}}

// acquireAdvisorSlot takes one of the database's slots without waiting; release must be called when true.
func acquireAdvisorSlot(db string) (release func(), ok bool) {
	advisorSlots.mu.Lock()
	ch := advisorSlots.by[db]
	if ch == nil {
		ch = make(chan struct{}, advisorMaxConcurrent)
		advisorSlots.by[db] = ch
	}
	advisorSlots.mu.Unlock()
	select {
	case ch <- struct{}{}:
		return func() { <-ch }, true
	default:
		return nil, false
	}
}

// advisorCacheKey separates external records by their connection details and update time.
func (rt *Router) advisorCacheKey(r *http.Request, label, containerID string) string {
	key := label + "|" + containerID + "|advisor"
	if containerID != "" || rt.externalDatabases == nil {
		return key
	}
	if rec, err := rt.externalDatabases.GetExternalDatabase(r.Context(), label); err == nil && rec != nil {
		key += fmt.Sprintf("|%s:%d|%s|%s|%s|%s|%s", rec.Host, rec.Port, rec.DatabaseName, rec.Username, rec.TLSMode, rec.NodeID, rec.UpdatedAt)
	}
	return key
}

// advisorTimedOut reports a check that hit its deadline; such a partial report is not worth caching.
func advisorTimedOut(rep dbadvisor.Report) bool {
	for _, f := range rep.Findings {
		if f.Kind == dbadvisor.KindCheckFailed && f.Evidence["reason"] == "timed out" {
			return true
		}
	}
	return false
}

// advisorResponse is the advisor report plus whether it came from the short cache.
type advisorResponse struct {
	dbadvisor.Report
	Cached bool `json:"cached,omitempty"`
}

// handleGetDatabaseAdvisor handles GET /api/v1/databases/{name}/advisor. It only reads statistics
// through the explorer's read-only exec path; suggested fixes are text and never run.
func (rt *Router) handleGetDatabaseAdvisor(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if desired, err := rt.databases.GetDesiredDatabase(r.Context(), name); err == nil && desired.Engine != store.EnginePostgres {
		writeError(w, http.StatusBadRequest, "performance advisor supports Postgres only (MySQL and MongoDB are not available yet), not "+desired.Engine)
		return
	}
	target, desired, ok := rt.viewerTarget(w, r, false)
	if !ok {
		return
	}
	if target.Dialect != dbviewer.DialectPostgres {
		writeError(w, http.StatusBadRequest, dbadvisor.ErrUnsupported.Error())
		return
	}
	cfg := rt.runtimeSettings.Advisor(r.Context())
	th := dbadvisor.Thresholds{
		UnusedIndexMinBytes: cfg.UnusedIndexMinBytes, SeqScanMinRows: cfg.SeqScanMinRows, SeqScanRatioPercent: cfg.SeqScanPercent,
		DeadTuplePercent: cfg.DeadTuplePercent, CacheHitMinPercent: cfg.CacheHitMinPercent, IdleInTxMinutes: cfg.IdleInTxMinutes,
		SlowStatementMs: cfg.SlowStatementMs, MaxFindings: cfg.MaxFindings, Timeout: cfg.Timeout,
	}
	label := desired.Name
	v, _, hit, err := viewerOverviews.do(cfg.CacheTTL, label, rt.advisorCacheKey(r, label, target.ContainerID), func() (any, error) {
		release, free := acquireAdvisorSlot(label)
		if !free {
			writeError(w, http.StatusTooManyRequests, "an advisor run for this database is already in progress, try again shortly")
			return nil, errViewerLoadFailed
		}
		defer release()
		rep, err := dbadvisor.Run(r.Context(), target, th, time.Now())
		if err != nil {
			// A caller that went away is not a server fault.
			if !errors.Is(err, context.Canceled) {
				rt.writeViewerError(w, "api: database advisor failed", label, err)
			}
			return nil, errViewerLoadFailed
		}
		if advisorTimedOut(rep) {
			// Invalidating while loading makes the cache skip storing this partial report.
			viewerOverviews.invalidate(label)
		}
		return rep, nil
	})
	if err != nil {
		if !errors.Is(err, errViewerLoadFailed) {
			rt.internalError(w, "api: database advisor failed", err, slog.String("name", label))
		}
		return
	}
	rep, _ := v.(dbadvisor.Report)
	writeJSON(w, http.StatusOK, advisorResponse{Report: rep, Cached: hit})
}
