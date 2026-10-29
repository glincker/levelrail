package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// AnalysisReportStore is the read surface the report routes need; *store.DB satisfies it.
type AnalysisReportStore interface {
	GetAnalysisReport(ctx context.Context, id string) (store.AnalysisReport, error)
	ListAnalysisReports(ctx context.Context, app string, limit int) ([]store.AnalysisReport, error)
}

// WithAnalysisReports enables the analysis report read routes.
func WithAnalysisReports(s AnalysisReportStore) Option {
	return func(rt *Router) { rt.analysisReports = s }
}

type analysisReportSummary struct {
	ID          string    `json:"id"`
	App         string    `json:"app"`
	TriggerKind string    `json:"trigger_kind"`
	TriggerID   string    `json:"trigger_id"`
	Status      string    `json:"status"`
	Summary     string    `json:"summary"`
	CreatedAt   time.Time `json:"created_at"`
}

type analysisReportDetail struct {
	analysisReportSummary
	SchemaVersion int             `json:"schema_version"`
	Report        json.RawMessage `json:"report"`
}

func toAnalysisSummary(r store.AnalysisReport) analysisReportSummary {
	var head struct {
		Summary string `json:"summary"`
	}
	_ = json.Unmarshal(r.Report, &head)
	return analysisReportSummary{ID: r.ID, App: r.App, TriggerKind: r.TriggerKind, TriggerID: r.TriggerID, Status: r.Status, Summary: head.Summary, CreatedAt: r.CreatedAt}
}

func (rt *Router) analysisReportResource(r *http.Request) string {
	if rt.analysisReports == nil {
		return ""
	}
	rep, err := rt.analysisReports.GetAnalysisReport(r.Context(), r.PathValue("id"))
	if err != nil {
		return ""
	}
	return "app:" + rep.App
}

// handleListAnalysisReports handles GET /api/v1/apps/{name}/analysis/reports, newest first.
func (rt *Router) handleListAnalysisReports(w http.ResponseWriter, r *http.Request) {
	if rt.analysisReports == nil {
		writeError(w, http.StatusNotImplemented, "analysis reports are not configured on this control plane")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := rt.analysisReports.ListAnalysisReports(r.Context(), r.PathValue("name"), limit)
	if err != nil {
		rt.internalError(w, "api: list analysis reports failed", err)
		return
	}
	out := make([]analysisReportSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, toAnalysisSummary(row))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetAnalysisReport handles GET /api/v1/analysis/reports/{id}.
func (rt *Router) handleGetAnalysisReport(w http.ResponseWriter, r *http.Request) {
	if rt.analysisReports == nil {
		writeError(w, http.StatusNotImplemented, "analysis reports are not configured on this control plane")
		return
	}
	row, err := rt.analysisReports.GetAnalysisReport(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrAnalysisReportNotFound) {
		writeError(w, http.StatusNotFound, "analysis report not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get analysis report failed", err)
		return
	}
	writeJSON(w, http.StatusOK, analysisReportDetail{analysisReportSummary: toAnalysisSummary(row), SchemaVersion: row.SchemaVersion, Report: row.Report})
}
