package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/backup"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

// AppVolumeMoveStore is the store surface handleMoveAppWithVolumes and its
// history endpoints need: recording a "move with volumes" attempt's steps
// and final outcome as it happens, so a caller can see exactly how far a
// partially-failed move got instead of a single black-box result.
// *store.DB satisfies this structurally.
type AppVolumeMoveStore interface {
	StartAppVolumeMove(ctx context.Context, m store.AppVolumeMove) error
	UpdateAppVolumeMoveSteps(ctx context.Context, id string, steps []store.AppVolumeMoveStep) error
	FinishAppVolumeMove(ctx context.Context, id, status, errMsg, finishedAt string) error
	GetAppVolumeMove(ctx context.Context, id string) (store.AppVolumeMove, error)
	ListAppVolumeMoves(ctx context.Context, serviceName string) ([]store.AppVolumeMove, error)
}

// appVolumeMoveStepResource mirrors store.AppVolumeMoveStep.
type appVolumeMoveStepResource struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	Error      string `json:"error,omitempty"`
	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at,omitempty"`
}

// appVolumeMoveResource is the wire shape for one "move with volumes"
// attempt, returned by handleMoveAppWithVolumes (202, steps still empty)
// and both history endpoints below (steps filled in as the background
// goroutine progresses).
type appVolumeMoveResource struct {
	ID          string                      `json:"id"`
	ServiceName string                      `json:"service_name"`
	FromNodeID  string                      `json:"from_node_id"`
	ToNodeID    string                      `json:"to_node_id"`
	Status      string                      `json:"status"`
	Error       string                      `json:"error,omitempty"`
	Steps       []appVolumeMoveStepResource `json:"steps"`
	StartedAt   string                      `json:"started_at"`
	FinishedAt  string                      `json:"finished_at,omitempty"`
}

func toAppVolumeMoveResource(m store.AppVolumeMove) appVolumeMoveResource {
	steps := make([]appVolumeMoveStepResource, len(m.Steps))
	for i, s := range m.Steps {
		steps[i] = appVolumeMoveStepResource{
			Name: s.Name, Status: s.Status, Error: s.Error,
			StartedAt: s.StartedAt, FinishedAt: s.FinishedAt,
		}
	}
	return appVolumeMoveResource{
		ID: m.ID, ServiceName: m.ServiceName, FromNodeID: m.FromNodeID, ToNodeID: m.ToNodeID,
		Status: m.Status, Error: m.Error, Steps: steps, StartedAt: m.StartedAt, FinishedAt: m.FinishedAt,
	}
}

// moveAppWithVolumesRequest is POST /api/v1/apps/{name}/move-with-volumes's
// body, the same node_id shape setAppNodeRequest already establishes.
type moveAppWithVolumesRequest struct {
	NodeID string `json:"node_id"`
}

// handleMoveAppWithVolumes handles
// POST /api/v1/apps/{name}/move-with-volumes: always responds with an
// appVolumeMoveResource so a caller never branches on shape, only status
// code (200 already-succeeded, or 202 poll GET .../moves/{id}). No
// volumes, or already on the target node, takes the synchronous
// applyPlainNodeMove path instead of standing up a background job with
// nothing real to do.
func (rt *Router) handleMoveAppWithVolumes(w http.ResponseWriter, r *http.Request) {
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "node-to-node volume moves are not configured on this control plane (no node runtime resolver)")
		return
	}

	name := r.PathValue("name")

	var req moveAppWithVolumesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := rt.validatePlacementTarget(r.Context(), req.NodeID); err != nil {
		rt.respondPlacementValidationError(w, err, req.NodeID, "api: move app with volumes: look up node failed")
		return
	}

	existing, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: move app with volumes: load existing failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	fromNodeID := existing.NodeID
	moveID, err := randomAppVolumeMoveID()
	if err != nil {
		rt.logger.Error("api: move app with volumes: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	startedAt := time.Now().UTC().Format(time.RFC3339)
	if err := rt.appVolumeMoves.StartAppVolumeMove(r.Context(), store.AppVolumeMove{
		ID: moveID, ServiceName: name, FromNodeID: fromNodeID, ToNodeID: req.NodeID, StartedAt: startedAt,
	}); err != nil {
		rt.logger.Error("api: move app with volumes: start history failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if len(existing.Volumes) == 0 || fromNodeID == req.NodeID {
		steps := &appVolumeMoveSteps{rt: rt, ctx: r.Context(), moveID: moveID}
		finish := steps.start("update_placement")
		if err := rt.applyPlainNodeMove(r.Context(), name, fromNodeID, req.NodeID); err != nil {
			finish(err)
			rt.finishAppVolumeMove(r.Context(), moveID, err)
			rt.logger.Error("api: move app with volumes: plain move failed", slog.String("error", err.Error()), slog.String("name", name))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		finish(nil)
		rt.finishAppVolumeMove(r.Context(), moveID, nil)
		rt.nudgeReconciler()

		final, err := rt.appVolumeMoves.GetAppVolumeMove(r.Context(), moveID)
		if err != nil {
			rt.logger.Error("api: move app with volumes: reload after move failed", slog.String("error", err.Error()), slog.String("move_id", moveID))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		writeJSON(w, http.StatusOK, toAppVolumeMoveResource(final))
		return
	}

	volumeNames := make([]string, len(existing.Volumes))
	for i, v := range existing.Volumes {
		volumeNames[i] = v.Name
	}

	go func() { //nolint:gosec // deliberately not r.Context(): it is cancelled the moment this handler returns, the same reasoning handleTriggerVolumeBackup's own goroutine gives
		rt.runAppVolumeMove(context.Background(), moveID, name, fromNodeID, req.NodeID, volumeNames)
	}()

	writeJSON(w, http.StatusAccepted, appVolumeMoveResource{
		ID: moveID, ServiceName: name, FromNodeID: fromNodeID, ToNodeID: req.NodeID,
		Status: store.BackupStatusRunning, Steps: []appVolumeMoveStepResource{}, StartedAt: startedAt,
	})
}

// appVolumeMoveSteps accumulates store.AppVolumeMoveStep entries and
// persists the full list after every change, so GetAppVolumeMove reflects
// live progress rather than only a result recorded at the very end.
type appVolumeMoveSteps struct {
	rt     *Router
	ctx    context.Context
	moveID string
	steps  []store.AppVolumeMoveStep
}

// start appends a new running step and persists it, returning a finish
// func the caller invokes with the step's outcome (nil for success).
func (s *appVolumeMoveSteps) start(name string) func(err error) {
	idx := len(s.steps)
	s.steps = append(s.steps, store.AppVolumeMoveStep{
		Name: name, Status: store.BackupStatusRunning, StartedAt: time.Now().UTC().Format(time.RFC3339),
	})
	s.persist()
	return func(err error) {
		s.steps[idx].FinishedAt = time.Now().UTC().Format(time.RFC3339)
		if err != nil {
			s.steps[idx].Status = store.BackupStatusFailed
			s.steps[idx].Error = err.Error()
		} else {
			s.steps[idx].Status = store.BackupStatusSucceeded
		}
		s.persist()
	}
}

func (s *appVolumeMoveSteps) persist() {
	if err := s.rt.appVolumeMoves.UpdateAppVolumeMoveSteps(s.ctx, s.moveID, s.steps); err != nil {
		s.rt.logger.Error("api: move app with volumes: persist step failed", slog.String("error", err.Error()), slog.String("move_id", s.moveID))
	}
}

// runAppVolumeMove is handleMoveAppWithVolumes' actual work: stop, move
// each volume (internal/backup.MoveVolume, no S3 target needed), flip
// node_id, resume. Every step's outcome is persisted before the next
// (appVolumeMoveSteps), so a poller sees exactly how far it got.
//
// Not atomic: a failed step leaves the app suspended rather than guessed
// back to running, and node_id changes only after every volume has
// already copied. Always safe to retry, though: Archive never touches
// the source, Restore always fully overwrites the destination.
func (rt *Router) runAppVolumeMove(ctx context.Context, moveID, serviceName, fromNodeID, toNodeID string, volumeNames []string) {
	steps := &appVolumeMoveSteps{rt: rt, ctx: ctx, moveID: moveID}

	srcRuntime, err := rt.execRuntime(fromNodeID)
	if err != nil {
		rt.finishAppVolumeMove(ctx, moveID, fmt.Errorf("resolve source node runtime: %w", err))
		return
	}
	if rt.runMoveStep(ctx, steps, moveID, "stop_app", func() error {
		if err := rt.apps.UpdateServiceSuspended(ctx, serviceName, true); err != nil {
			return err
		}
		return application.New(serviceName, rt.apps, srcRuntime).Teardown(ctx)
	}) {
		return
	}

	dstRuntime, err := rt.execRuntime(toNodeID)
	if err != nil {
		rt.finishAppVolumeMove(ctx, moveID, fmt.Errorf("resolve destination node runtime: %w", err))
		return
	}
	archiver := &backup.ContainerVolumeArchiver{Runtime: srcRuntime}
	restorer := &backup.ContainerVolumeRestorer{Runtime: dstRuntime}

	for _, volumeName := range volumeNames {
		volumeName := volumeName
		if rt.runMoveStep(ctx, steps, moveID, "move_volume:"+volumeName, func() error {
			return backup.MoveVolume(ctx, archiver, restorer, volumeName)
		}) {
			return
		}
	}

	if rt.runMoveStep(ctx, steps, moveID, "update_placement", func() error {
		return rt.apps.UpdateServiceNode(ctx, serviceName, toNodeID)
	}) {
		return
	}

	if rt.runMoveStep(ctx, steps, moveID, "resume_app", func() error {
		if err := rt.apps.UpdateServiceSuspended(ctx, serviceName, false); err != nil {
			return err
		}
		rt.nudgeReconciler()
		return nil
	}) {
		return
	}

	rt.finishAppVolumeMove(ctx, moveID, nil)
}

// runMoveStep runs one step of runAppVolumeMove: starts it, runs op, records
// success or failure. On failure it also finishes the whole move (the
// caller's own signal to stop) and returns true. Collapses the repeated
// "start, run, record, maybe abort" shape every step in runAppVolumeMove
// otherwise needs on its own.
func (rt *Router) runMoveStep(ctx context.Context, steps *appVolumeMoveSteps, moveID, name string, op func() error) (aborted bool) {
	finish := steps.start(name)
	if err := op(); err != nil {
		finish(err)
		rt.finishAppVolumeMove(ctx, moveID, err)
		return true
	}
	finish(nil)
	return false
}

func (rt *Router) finishAppVolumeMove(ctx context.Context, moveID string, runErr error) {
	status := store.BackupStatusSucceeded
	errMsg := ""
	if runErr != nil {
		status = store.BackupStatusFailed
		errMsg = runErr.Error()
		rt.logger.Error("api: move app with volumes failed", slog.String("error", runErr.Error()), slog.String("move_id", moveID))
	}
	finishedAt := time.Now().UTC().Format(time.RFC3339)
	if err := rt.appVolumeMoves.FinishAppVolumeMove(ctx, moveID, status, errMsg, finishedAt); err != nil {
		rt.logger.Error("api: move app with volumes: finish history failed", slog.String("error", err.Error()), slog.String("move_id", moveID))
	}
}

// handleListAppVolumeMoves handles GET /api/v1/apps/{name}/moves: every
// "move with volumes" attempt for name, newest first.
func (rt *Router) handleListAppVolumeMoves(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if _, err := rt.apps.GetDesiredService(r.Context(), name); errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	} else if err != nil {
		rt.logger.Error("api: list app volume moves: load service failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	moves, err := rt.appVolumeMoves.ListAppVolumeMoves(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: list app volume moves failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]appVolumeMoveResource, 0, len(moves))
	for _, m := range moves {
		out = append(out, toAppVolumeMoveResource(m))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetAppVolumeMove handles GET /api/v1/apps/{name}/moves/{id}: the
// single-attempt counterpart of handleListAppVolumeMoves, what the UI and
// CLI poll while a move is still running.
func (rt *Router) handleGetAppVolumeMove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")

	m, err := rt.appVolumeMoves.GetAppVolumeMove(r.Context(), id)
	if errors.Is(err, store.ErrAppVolumeMoveNotFound) {
		writeError(w, http.StatusNotFound, "move not found")
		return
	} else if err != nil {
		rt.logger.Error("api: get app volume move failed", slog.String("error", err.Error()), slog.String("move_id", id))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if m.ServiceName != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("move %q was not for app %q", id, name))
		return
	}
	writeJSON(w, http.StatusOK, toAppVolumeMoveResource(m))
}

func randomAppVolumeMoveID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate app volume move id: %w", err)
	}
	return "avm_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
