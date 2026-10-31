package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/analysis"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeAnalysisRunner struct {
	got  analysis.Trigger
	busy bool
}

func (f *fakeAnalysisRunner) GenerateManual(_ context.Context, tr analysis.Trigger) (store.AnalysisReport, error) {
	f.got = tr
	if f.busy {
		return store.AnalysisReport{}, analysis.ErrBusy
	}
	return store.AnalysisReport{ID: "ar_1", App: tr.App, TriggerKind: tr.Kind, Status: "complete", Report: []byte(`{"summary":"ok"}`)}, nil
}

func TestRunAnalysisReport(t *testing.T) {
	rt := &Router{}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/analysis/reports", nil)
	req.SetPathValue("name", "web")

	rec := httptest.NewRecorder()
	rt.handleRunAnalysisReport(rec, req)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("disabled: got %d, want 501", rec.Code)
	}

	f := &fakeAnalysisRunner{}
	rt.SetAnalysisRunner(f)
	rec = httptest.NewRecorder()
	rt.handleRunAnalysisReport(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("enabled: got %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if f.got.App != "web" || f.got.Kind != analysis.TriggerManual {
		t.Fatalf("trigger = %+v", f.got)
	}
}

func TestRunAnalysisReportBusy(t *testing.T) {
	rt := &Router{}
	rt.SetAnalysisRunner(&fakeAnalysisRunner{busy: true})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/web/analysis/reports", nil)
	req.SetPathValue("name", "web")
	rec := httptest.NewRecorder()
	rt.handleRunAnalysisReport(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("busy: got %d, want 429", rec.Code)
	}
}
