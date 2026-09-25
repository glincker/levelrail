package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/pipeline"
	"github.com/GLINCKER/levelrail/internal/store"
)

const pipelineSyncTimeout = 2 * time.Minute

// PipelineSyncer copies an app's repository pipeline files into its saved
// definitions. *pipeline.Syncer satisfies it.
type PipelineSyncer interface {
	Sync(ctx context.Context, app string) (pipeline.SyncResult, error)
	SyncOnPush(ctx context.Context, app, ref string) (pipeline.SyncResult, bool, error)
}

// PipelineSyncStore holds the per-app sync settings. *store.DB satisfies it.
type PipelineSyncStore interface {
	GetPipelineSync(ctx context.Context, app string) (store.PipelineSync, error)
	SavePipelineSync(ctx context.Context, s store.PipelineSync) error
}

type pipelineSyncWiring struct {
	syncer PipelineSyncer
	store  PipelineSyncStore
}

// SetPipelineSync enables repository sync of pipeline files: on a push to
// the tracked branch, and on demand through /pipeline-sync.
func (rt *Router) SetPipelineSync(syncer PipelineSyncer, st PipelineSyncStore) {
	rt.pipelineSync = &pipelineSyncWiring{syncer: syncer, store: st}
}

func (rt *Router) syncReady(w http.ResponseWriter) bool {
	if !rt.pipelinesReady(w) {
		return false
	}
	if rt.pipelineSync == nil {
		writeError(w, http.StatusNotImplemented, "repository sync is not configured on this control plane")
		return false
	}
	return true
}

type pipelineSyncStatus struct {
	// Connected is false when the app has no git repository to sync from.
	Connected    bool       `json:"connected"`
	RepoIsTruth  bool       `json:"repo_is_truth"`
	LastSHA      string     `json:"last_sha,omitempty"`
	LastSyncedAt *time.Time `json:"last_synced_at,omitempty"`
	LastError    string     `json:"last_error,omitempty"`
}

func (rt *Router) pipelineSyncStatusFor(ctx context.Context, app string) (pipelineSyncStatus, error) {
	st, err := rt.pipelineSync.store.GetPipelineSync(ctx, app)
	if err != nil {
		return pipelineSyncStatus{}, err
	}
	out := pipelineSyncStatus{RepoIsTruth: st.RepoIsTruth, LastSHA: st.LastSHA, LastError: st.LastError}
	if !st.LastSyncAt.IsZero() {
		at := st.LastSyncAt
		out.LastSyncedAt = &at
	}
	if _, err := rt.gitSources.GetGitSource(ctx, app); err == nil {
		out.Connected = true
	} else if !errors.Is(err, store.ErrGitSourceNotFound) {
		return pipelineSyncStatus{}, err
	}
	return out, nil
}

// handleGetPipelineSync handles GET /api/v1/apps/{name}/pipeline-sync.
func (rt *Router) handleGetPipelineSync(w http.ResponseWriter, r *http.Request) {
	if !rt.syncReady(w) || !rt.requireApp(w, r) {
		return
	}
	out, err := rt.pipelineSyncStatusFor(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.internalError(w, "api: get pipeline sync failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type pipelineSyncSettings struct {
	RepoIsTruth bool `json:"repo_is_truth"`
}

// handleSetPipelineSync handles PUT /api/v1/apps/{name}/pipeline-sync.
func (rt *Router) handleSetPipelineSync(w http.ResponseWriter, r *http.Request) {
	if !rt.syncReady(w) || !rt.requireApp(w, r) {
		return
	}
	var req pipelineSyncSettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, pipelineBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	app := r.PathValue("name")
	cur, err := rt.pipelineSync.store.GetPipelineSync(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: get pipeline sync failed", err)
		return
	}
	cur.RepoIsTruth = req.RepoIsTruth
	if err := rt.pipelineSync.store.SavePipelineSync(r.Context(), cur); err != nil {
		rt.internalError(w, "api: save pipeline sync failed", err)
		return
	}
	out, err := rt.pipelineSyncStatusFor(r.Context(), app)
	if err != nil {
		rt.internalError(w, "api: get pipeline sync failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// handleRunPipelineSync handles POST /api/v1/apps/{name}/pipeline-sync: a
// sync now of the tracked branch.
func (rt *Router) handleRunPipelineSync(w http.ResponseWriter, r *http.Request) {
	if !rt.syncReady(w) || !rt.requireApp(w, r) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), pipelineSyncTimeout)
	defer cancel()
	res, err := rt.pipelineSync.syncer.Sync(ctx, r.PathValue("name"))
	if errors.Is(err, pipeline.ErrNoRepo) {
		writeError(w, http.StatusConflict, "connect a git repository to this app before syncing pipelines")
		return
	}
	if err != nil {
		rt.logger.Warn("api: pipeline sync failed", slog.String("app", r.PathValue("name")), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type pipelineTriggerView struct {
	ID        int64     `json:"id"`
	Pipeline  string    `json:"pipeline,omitempty"`
	Event     string    `json:"event"`
	Ref       string    `json:"ref,omitempty"`
	SHA       string    `json:"sha,omitempty"`
	Decision  string    `json:"decision"`
	Reason    string    `json:"reason"`
	RunID     string    `json:"run_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// handleListPipelineTriggers handles GET /api/v1/apps/{name}/pipeline-triggers:
// why recent git events did or did not start a run.
func (rt *Router) handleListPipelineTriggers(w http.ResponseWriter, r *http.Request) {
	if !rt.pipelinesReady(w) || !rt.requireApp(w, r) {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	rows, err := rt.pipelineStore.ListPipelineTriggerLog(r.Context(), r.PathValue("name"), limit)
	if err != nil {
		rt.internalError(w, "api: list pipeline trigger log failed", err)
		return
	}
	out := make([]pipelineTriggerView, 0, len(rows))
	for _, e := range rows {
		out = append(out, pipelineTriggerView{ID: e.ID, Pipeline: e.Pipeline, Event: e.Event, Ref: e.Ref, SHA: e.SHA,
			Decision: e.Decision, Reason: e.Reason, RunID: e.RunID, CreatedAt: e.CreatedAt})
	}
	writeJSON(w, http.StatusOK, out)
}

type pipelineHoldDecision struct {
	Decision string `json:"decision"`
}

// handleDecidePipelineRunHold handles POST .../pipeline-runs/{id}/hold: an
// approver releases or rejects a run held for approval, such as a pull
// request from a fork. The route is gated at deploy.
func (rt *Router) handleDecidePipelineRunHold(w http.ResponseWriter, r *http.Request) {
	if !rt.runnerReady(w) {
		return
	}
	run, ok := rt.loadOwnedPipelineRun(w, r)
	if !ok {
		return
	}
	var req pipelineHoldDecision
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, pipelineBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Decision != "approved" && req.Decision != "rejected" {
		writeError(w, http.StatusBadRequest, `decision must be "approved" or "rejected"`)
		return
	}
	applied, err := rt.pipelineRunner.DecideHold(r.Context(), run.ID, req.Decision == "approved", rt.pipelineActor(r))
	if err != nil {
		rt.internalError(w, "api: decide pipeline run hold failed", err)
		return
	}
	if !applied {
		writeError(w, http.StatusConflict, "this run is not waiting for approval")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"decision": req.Decision})
}
