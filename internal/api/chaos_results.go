package api

import (
	"context"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/chaos"
)

const (
	maxChaosBodyBytes   = 256 << 10
	chaosReportRowLimit = 200
	maxChaosBatch       = 20
)

// ChaosStore persists measured chaos scenario runs.
type ChaosStore interface {
	RecordChaosResult(ctx context.Context, r chaos.Result, recordedBy string) (string, error)
	ListChaosResults(ctx context.Context, limit int) ([]chaos.Result, error)
}

type chaosReport struct {
	Scenarios  []chaos.Info   `json:"scenarios"`
	Latest     []chaos.Result `json:"latest"`
	Unmeasured []string       `json:"not_yet_measured"`
}

type chaosRecordRequest struct {
	Results []chaos.Result `json:"results"`
}

type chaosRecordResponse struct {
	IDs []string `json:"ids"`
}

func (rt *Router) handleChaosReport(w http.ResponseWriter, r *http.Request) {
	if rt.chaosResults == nil {
		writeError(w, http.StatusNotImplemented, "the resilience report is not available on this control plane")
		return
	}
	rows, err := rt.chaosResults.ListChaosResults(r.Context(), chaosReportRowLimit)
	if err != nil {
		rt.internalError(w, "api: list chaos results", err)
		return
	}
	latest, unmeasured := chaos.Latest(rows)
	if latest == nil {
		latest = []chaos.Result{}
	}
	if unmeasured == nil {
		unmeasured = []string{}
	}
	writeJSON(w, http.StatusOK, chaosReport{Scenarios: chaos.Catalog(), Latest: latest, Unmeasured: unmeasured})
}

func (rt *Router) handleRecordChaosResults(w http.ResponseWriter, r *http.Request) {
	if rt.chaosResults == nil {
		writeError(w, http.StatusNotImplemented, "the resilience report is not available on this control plane")
		return
	}
	var req chaosRecordRequest
	if err := decodeJSONBodyLimit(r, &req, maxChaosBodyBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Results) == 0 || len(req.Results) > maxChaosBatch {
		writeError(w, http.StatusBadRequest, "send between 1 and 20 results")
		return
	}
	for _, res := range req.Results {
		if !chaos.Valid(res.Scenario) {
			writeError(w, http.StatusBadRequest, "unknown scenario "+res.Scenario)
			return
		}
		if res.Status != chaos.StatusPassed && res.Status != chaos.StatusFailed && res.Status != chaos.StatusSkipped {
			writeError(w, http.StatusBadRequest, "status must be passed, failed or skipped")
			return
		}
	}
	_, _, actor, _ := rt.currentActor(r)
	ids := make([]string, 0, len(req.Results))
	for _, res := range req.Results {
		id, err := rt.chaosResults.RecordChaosResult(r.Context(), res, actor)
		if err != nil {
			rt.internalError(w, "api: record chaos result", err)
			return
		}
		ids = append(ids, id)
	}
	writeJSON(w, http.StatusCreated, chaosRecordResponse{IDs: ids})
}
