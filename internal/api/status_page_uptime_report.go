package api

import (
	"context"
	"net/http"
	"time"

	"github.com/glincker/levelrail/internal/statuspage"
	"github.com/glincker/levelrail/internal/store"
)

// statusDailySource is optional on the store so existing fakes keep working.
type statusDailySource interface {
	ListStatusDaily(ctx context.Context, sinceDay string) ([]store.StatusDaily, error)
}

type uptimeReportResource struct {
	Month string                       `json:"month"`
	Rows  []statuspage.UptimeReportRow `json:"rows"`
}

// handleStatusUptimeReport handles GET /api/v1/status-page/uptime-report. It
// is signed-in only: rows carry app names the public page never shows.
func (rt *Router) handleStatusUptimeReport(w http.ResponseWriter, r *http.Request) {
	if !rt.statusPageReady(w) {
		return
	}
	src, ok := rt.statusPage.(statusDailySource)
	if !ok {
		writeError(w, http.StatusNotImplemented, "uptime history is not available")
		return
	}
	month := r.URL.Query().Get("month")
	if month == "" {
		month = time.Now().UTC().Format("2006-01")
	}
	comps, err := rt.statusPage.ListStatusComponents(r.Context())
	if err != nil {
		rt.internalError(w, "api: uptime report components failed", err)
		return
	}
	daily, err := src.ListStatusDaily(r.Context(), month+"-01")
	if err != nil {
		rt.internalError(w, "api: uptime report daily failed", err)
		return
	}
	rows, err := statuspage.MonthlyReport(comps, daily, month)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, uptimeReportResource{Month: month, Rows: rows})
}
