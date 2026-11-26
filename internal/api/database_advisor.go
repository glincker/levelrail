package api

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbadvisor"
	"github.com/GLINCKER/levelrail/internal/dbviewer"
	"github.com/GLINCKER/levelrail/internal/store"
)

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
	v, _, hit, err := viewerOverviews.do(cfg.CacheTTL, label, label+"|"+target.ContainerID+"|advisor", func() (any, error) {
		rep, err := dbadvisor.Run(r.Context(), target, th, time.Now())
		if err != nil {
			rt.writeViewerError(w, "api: database advisor failed", label, err)
			return nil, errViewerLoadFailed
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
