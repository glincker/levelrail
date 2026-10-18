package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	pipelineFixLogPages    = 5
	pipelineFixLogPageSize = 1000
	pipelineInsightWindow  = 30
)

// handleListPipelineTemplates handles GET /api/v1/pipelines/templates.
func (rt *Router) handleListPipelineTemplates(w http.ResponseWriter, _ *http.Request) {
	list := pipeline.Catalog()
	for i := range list {
		list[i].YAML = ""
	}
	writeJSON(w, http.StatusOK, list)
}

// handleGetPipelineTemplate handles GET /api/v1/pipelines/templates/{id}.
func (rt *Router) handleGetPipelineTemplate(w http.ResponseWriter, r *http.Request) {
	t, err := pipeline.CatalogByID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (rt *Router) failedJobOutput(ctx context.Context, runID, jobKey string) (string, error) {
	var b strings.Builder
	var after int64
	for range pipelineFixLogPages {
		lines, err := rt.pipelineStore.ListPipelineLogs(ctx, runID, jobKey, after, pipelineFixLogPageSize)
		if err != nil {
			return "", err
		}
		for _, l := range lines {
			b.WriteString(l.Line)
			b.WriteByte('\n')
			after = l.ID
		}
		if len(lines) < pipelineFixLogPageSize {
			break
		}
	}
	return b.String(), nil
}

func baseJobKey(k string) string {
	if i := strings.IndexByte(k, '['); i >= 0 {
		return k[:i]
	}
	return k
}

// handleListPipelineRunFixes handles GET .../pipeline-runs/{id}/fixes: typed
// suggestions for each failed job, matched from known error patterns.
func (rt *Router) handleListPipelineRunFixes(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	jobs, err := rt.pipelineStore.ListPipelineJobs(r.Context(), run.ID)
	if err != nil {
		rt.internalError(w, "api: list pipeline jobs failed", err)
		return
	}
	out := []pipeline.Fix{}
	seen := map[string]bool{}
	for _, j := range jobs {
		if j.Status != store.PipelineStatusFailed {
			continue
		}
		text, err := rt.failedJobOutput(r.Context(), run.ID, j.Key)
		if err != nil {
			rt.internalError(w, "api: read pipeline logs failed", err)
			return
		}
		for _, f := range pipeline.SuggestFixes(baseJobKey(j.Key), text+"\n"+j.Reason) {
			id := f.Job + "/" + f.Code + "/" + f.Kind
			if !seen[id] {
				seen[id] = true
				out = append(out, f)
			}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type applyPipelineFixRequest struct {
	Kind  string `json:"kind"`
	Job   string `json:"job"`
	Rerun bool   `json:"rerun"`
}

type applyPipelineFixResponse struct {
	Pipeline string `json:"pipeline"`
	Run      string `json:"run,omitempty"`
}

// handleApplyPipelineRunFix handles POST .../pipeline-runs/{id}/fixes/apply.
// It edits the run's pipeline file, and can start a fresh run from the result.
func (rt *Router) handleApplyPipelineRunFix(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	var req applyPipelineFixRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, pipelineBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	pipes, err := rt.pipelineStore.ListPipelines(r.Context(), run.AppName)
	if err != nil {
		rt.internalError(w, "api: list pipelines failed", err)
		return
	}
	var p store.Pipeline
	found := false
	for _, c := range pipes {
		if c.ID == run.PipelineID {
			p, found = c, true
		}
	}
	if !found {
		writeError(w, http.StatusNotFound, "pipeline not found")
		return
	}
	if p.Source == pipeline.SourceRepo {
		writeError(w, http.StatusConflict, "this pipeline is synced from the repository, edit the file there")
		return
	}
	edited, err := pipeline.ApplyFix([]byte(p.YAML), req.Job, req.Kind)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, issues := pipeline.Validate(edited); len(issues) > 0 {
		rt.writePipelineIssues(w, issues)
		return
	}
	p.YAML = string(edited)
	p.UpdatedAt = time.Now().UTC()
	saved, err := rt.pipelineStore.SavePipeline(r.Context(), p)
	if err != nil {
		rt.internalError(w, "api: save fixed pipeline failed", err)
		return
	}
	res := applyPipelineFixResponse{Pipeline: saved.Name}
	if req.Rerun && rt.pipelineRunner != nil && saved.Enabled {
		var inputs map[string]string
		_ = json.Unmarshal([]byte(run.InputsJSON), &inputs)
		nr, err := rt.pipelineRunner.Start(r.Context(), saved, pipeline.StartOptions{Trigger: pipeline.TriggerManual, Actor: rt.pipelineActor(r), Ref: run.Ref, SHA: run.CommitSHA, Inputs: inputs})
		if err != nil && !errors.Is(err, pipeline.ErrInvalidInput) {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err == nil {
			res.Run = nr.ID
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// handlePipelineInsights handles GET .../pipelines/{pname}/insights.
func (rt *Router) handlePipelineInsights(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) {
		return
	}
	p, ok := rt.loadPipeline(w, r)
	if !ok {
		return
	}
	runs, err := rt.pipelineStore.ListPipelineRuns(r.Context(), p.AppName, p.ID, pipelineInsightWindow)
	if err != nil {
		rt.internalError(w, "api: list pipeline runs failed", err)
		return
	}
	samples := make([]pipeline.RunSample, 0, len(runs))
	for _, run := range runs {
		if run.Status != store.PipelineStatusSucceeded && run.Status != store.PipelineStatusFailed {
			continue
		}
		jobs, err := rt.pipelineStore.ListPipelineJobs(r.Context(), run.ID)
		if err != nil {
			rt.internalError(w, "api: list pipeline jobs failed", err)
			return
		}
		s := pipeline.RunSample{RunID: run.ID, Number: run.Number, CommitSHA: run.CommitSHA}
		for _, j := range jobs {
			var d int64
			if j.StartedAt != nil && j.FinishedAt != nil {
				d = int64(j.FinishedAt.Sub(*j.StartedAt).Seconds())
			}
			s.Jobs = append(s.Jobs, pipeline.JobSample{Key: j.Key, Status: j.Status, DurationSeconds: d, Attempt: j.Attempt})
		}
		samples = append(samples, s)
	}
	writeJSON(w, http.StatusOK, pipeline.ComputeInsights(samples))
}
