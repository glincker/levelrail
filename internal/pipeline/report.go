package pipeline

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Commit status states a run is reported as.
const (
	ReportPending = "pending"
	ReportSuccess = "success"
	ReportFailure = "failure"
	ReportError   = "error"
)

// ErrNoReportTarget is returned by a StatusReporter when the run's app has
// no git forge to report to. The engine treats it as "nothing to do".
var ErrNoReportTarget = errors.New("pipeline: no git forge to report to")

const defaultReportTimeout = 10 * time.Second

// ReportRequest is one commit status to post for a run.
type ReportRequest struct {
	App         string
	Pipeline    string
	RunID       string
	RunNumber   int
	SHA         string
	Ref         string
	State       string
	Description string
	// Context labels the status so several pipelines stay distinct.
	Context string
}

// ReportReceipt says where a status went.
type ReportReceipt struct {
	Provider string
	// URL is the dashboard page the status links to, empty when the
	// control plane has no dashboard URL configured.
	URL string
}

// StatusReporter posts a run's state to the git forge that hosts its
// commit. Implementations return ErrNoReportTarget when there is no forge.
type StatusReporter interface {
	ReportRun(ctx context.Context, req ReportRequest) (ReportReceipt, error)
}

type reportRecorder interface {
	SetPipelineRunReport(ctx context.Context, id, provider, state, url, warning string) error
}

// reportRun posts state for run to its git forge. A failure is logged and
// recorded on the run as a warning; it never changes the run's outcome.
func (e *Engine) reportRun(ctx context.Context, run store.PipelineRun, def *Definition, state, description string) {
	if e.cfg.Reporter == nil || run.CommitSHA == "" || !def.ReportsStatus() {
		return
	}
	name := def.Name
	if p, err := e.cfg.Store.GetPipeline(ctx, run.PipelineID); err == nil {
		name = p.Name
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), e.cfg.ReportTimeout)
	defer cancel()
	receipt, err := e.cfg.Reporter.ReportRun(ctx, ReportRequest{
		App: run.AppName, Pipeline: name, RunID: run.ID, RunNumber: run.Number, SHA: run.CommitSHA, Ref: run.Ref,
		State: state, Description: description, Context: fmt.Sprintf("%s/pipeline/%s", e.cfg.NamePrefix, name),
	})
	if errors.Is(err, ErrNoReportTarget) {
		return
	}
	warning := ""
	if err != nil {
		warning = "status not reported: " + err.Error()
		e.cfg.Logger.Warn("pipeline: report status failed", slog.String("run_id", run.ID), slog.String("app", run.AppName), slog.String("state", state), slog.String("error", err.Error()))
		state = ""
	}
	if rec, ok := e.cfg.Store.(reportRecorder); ok {
		if rerr := rec.SetPipelineRunReport(ctx, run.ID, receipt.Provider, state, receipt.URL, warning); rerr != nil {
			e.cfg.Logger.Warn("pipeline: record report outcome failed", slog.String("run_id", run.ID), slog.String("error", rerr.Error()))
		}
	}
}

func reportStateFor(status string) string {
	switch status {
	case store.PipelineStatusSucceeded:
		return ReportSuccess
	case store.PipelineStatusFailed:
		return ReportFailure
	}
	return ReportError
}
