package api

import (
	"net/http"
	"strconv"

	"github.com/GLINCKER/levelrail/internal/ingress"
)

// AccessSource reads recent edge requests for an app.
type AccessSource interface {
	AppAccess(app string, f ingress.AccessFilter) []ingress.AccessRecord
}

// WithAccessSource turns on GET /api/v1/apps/{name}/access.
func WithAccessSource(s AccessSource) Option {
	return func(rt *Router) { rt.access = s }
}

// EdgeAccess adapts the process-wide stats and access log to AccessSource.
type EdgeAccess struct {
	Stats *ingress.RequestStats
	Log   *ingress.AccessLog
}

// AppAccess implements AccessSource.
func (e EdgeAccess) AppAccess(app string, f ingress.AccessFilter) []ingress.AccessRecord {
	return e.Log.Query(e.Stats.KeysForApp(app), f)
}

func (rt *Router) handleAppAccess(w http.ResponseWriter, r *http.Request) {
	if rt.access == nil {
		writeError(w, http.StatusNotImplemented, "edge access log is not configured on this control plane")
		return
	}
	q := r.URL.Query()
	var f ingress.AccessFilter
	var err error
	if v := q.Get("min_ms"); v != "" {
		if f.MinMillis, err = strconv.ParseFloat(v, 64); err != nil || f.MinMillis < 0 {
			writeError(w, http.StatusBadRequest, "min_ms must be a non-negative number")
			return
		}
	}
	if v := q.Get("min_status"); v != "" {
		if f.MinStatus, err = strconv.Atoi(v); err != nil || f.MinStatus < 0 {
			writeError(w, http.StatusBadRequest, "min_status must be a non-negative integer")
			return
		}
	}
	if v := q.Get("limit"); v != "" {
		if f.Limit, err = strconv.Atoi(v); err != nil || f.Limit < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
	}
	if f.Since, err = parseOptionalTime(q.Get("since")); err != nil {
		writeError(w, http.StatusBadRequest, "since must be RFC3339")
		return
	}
	f.Path = q.Get("path")
	out := rt.access.AppAccess(r.PathValue("name"), f)
	if out == nil {
		out = []ingress.AccessRecord{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, out)
}
