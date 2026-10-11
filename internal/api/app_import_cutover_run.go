package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cutover"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Env knobs bounding a cutover run.
const (
	envCutoverHealthTimeout  = "APP_CUTOVER_HEALTH_TIMEOUT"
	envCutoverProbeTimeout   = "APP_CUTOVER_PROBE_TIMEOUT"
	envCutoverVerifyTimeout  = "APP_CUTOVER_VERIFY_TIMEOUT"
	envCutoverVerifyInterval = "APP_CUTOVER_VERIFY_INTERVAL"
	envCutoverResumeMaxAge   = "APP_CUTOVER_RESUME_MAX_AGE"
	cutoverRunListLimit      = 20
	cutoverWorkerStopWait    = 30 * time.Second
)

// cutoverService is the in-memory side of cutover runs: which runs have a
// worker and the test seams. Run state itself lives in the database.
type cutoverService struct {
	mu      sync.Mutex
	workers map[string]*cutoverWorker
	// cfg and httpProbe are test seams; zero values use env and defaults.
	cfg       *cutover.Config
	httpProbe func(ctx context.Context, rawURL, dialAddr string) (int, error)
}

type cutoverWorker struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func (rt *Router) cutoverConfig() cutover.Config {
	if rt.cutover.cfg != nil {
		return *rt.cutover.cfg
	}
	c := cutover.DefaultConfig()
	c.HealthTimeout = envDuration(envCutoverHealthTimeout, c.HealthTimeout)
	c.ProbeTimeout = envDuration(envCutoverProbeTimeout, c.ProbeTimeout)
	c.VerifyTimeout = envDuration(envCutoverVerifyTimeout, c.VerifyTimeout)
	c.VerifyInterval = envDuration(envCutoverVerifyInterval, c.VerifyInterval)
	c.ResumeMaxAge = envDuration(envCutoverResumeMaxAge, c.ResumeMaxAge)
	return c
}

// cutoverRunner builds a runner over the database, nil when the store does
// not support cutover runs.
func (rt *Router) cutoverRunner() *cutover.Runner {
	rows, ok := any(rt.appImports).(cutover.RunRows)
	if !ok {
		return nil
	}
	return &cutover.Runner{Store: cutover.DBStore{DB: rows}, Deps: rt.cutoverDeps(), Cfg: rt.cutoverConfig(), Log: rt.logger}
}

func (rt *Router) launchCutover(runner *cutover.Runner, id string) {
	ctx, cancel := context.WithCancel(context.Background())
	w := &cutoverWorker{cancel: cancel, done: make(chan struct{})}
	rt.cutover.mu.Lock()
	if rt.cutover.workers == nil {
		rt.cutover.workers = map[string]*cutoverWorker{}
	}
	if _, busy := rt.cutover.workers[id]; busy {
		rt.cutover.mu.Unlock()
		cancel()
		return
	}
	rt.cutover.workers[id] = w
	rt.cutover.mu.Unlock()
	go func() { //nolint:gosec // outlives the request on purpose: a switch waits minutes
		defer close(w.done)
		defer func() {
			rt.cutover.mu.Lock()
			delete(rt.cutover.workers, id)
			rt.cutover.mu.Unlock()
			cancel()
		}()
		run, err := runner.Execute(ctx, id)
		if err != nil {
			rt.logger.Warn("api: cutover: run stopped", slog.String("error", err.Error()), slog.String("run_id", id))
			return
		}
		rt.logger.Info("api: cutover: run finished", slog.String("run_id", id), slog.String("app", run.App), slog.String("state", run.State))
	}()
}

// stopCutoverWorker cancels the run's worker and waits for it to exit.
func (rt *Router) stopCutoverWorker(id string) {
	rt.cutover.mu.Lock()
	w := rt.cutover.workers[id]
	rt.cutover.mu.Unlock()
	if w == nil {
		return
	}
	w.cancel()
	select {
	case <-w.done:
	case <-time.After(cutoverWorkerStopWait):
	}
}

// ResumeCutoverRuns continues or rolls back cutover runs a restart
// interrupted. Call it once after the router is built.
func (rt *Router) ResumeCutoverRuns(ctx context.Context) {
	runner := rt.cutoverRunner()
	if runner == nil {
		return
	}
	if err := runner.Recover(ctx, func(id string) { rt.launchCutover(runner, id) }); err != nil {
		rt.logger.Error("api: cutover: resume failed", slog.String("error", err.Error()))
	}
}

type cutoverRunResource struct {
	cutover.Run
	Rollbackable bool `json:"rollbackable"`
	InFlight     bool `json:"in_flight"`
}

func toCutoverRunResource(r cutover.Run) cutoverRunResource {
	return cutoverRunResource{Run: r, Rollbackable: r.Rollbackable(), InFlight: r.InFlight()}
}

type cutoverRunsResponse struct {
	Runs []cutoverRunResource `json:"runs"`
}

func (rt *Router) loadCutoverItem(w http.ResponseWriter, r *http.Request) (store.AppImportSession, store.AppImportItem, bool) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return sess, store.AppImportItem{}, false
	}
	idx := findItem(items, r.PathValue("item"))
	if idx < 0 || !items[idx].Selected {
		writeError(w, http.StatusNotFound, "item not found")
		return sess, store.AppImportItem{}, false
	}
	return sess, items[idx], true
}

// handleAppImportCutoverPlan handles GET .../items/{item}/cutover/plan.
// Read-only: it reads DNS, images and app status and changes nothing.
func (rt *Router) handleAppImportCutoverPlan(w http.ResponseWriter, r *http.Request) {
	sess, it, ok := rt.loadCutoverItem(w, r)
	if !ok {
		return
	}
	plan, err := cutoverBackend{rt: rt}.Plan(r.Context(), cutover.PlanRequest{SessionID: sess.ID, SourceID: it.SourceID, App: it.TargetName, DNSWrite: rt.callerIsRoot(r)})
	if err != nil {
		rt.internalError(w, "api: cutover: plan failed", err, slog.String("session_id", sess.ID), slog.String("app", it.TargetName))
		return
	}
	plan.SessionID, plan.SourceID = sess.ID, it.SourceID
	writeJSON(w, http.StatusOK, plan)
}

type cutoverStartRequest struct {
	Mode string `json:"mode"`
	// Confirm must equal the app name for a switch.
	Confirm        string `json:"confirm,omitempty"`
	AcceptWarnings bool   `json:"accept_warnings,omitempty"`
}

// handleStartAppImportCutover handles POST .../items/{item}/cutover/runs.
func (rt *Router) handleStartAppImportCutover(w http.ResponseWriter, r *http.Request) {
	sess, it, ok := rt.loadCutoverItem(w, r)
	if !ok {
		return
	}
	var req cutoverStartRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Mode != cutover.ModeDryRun && req.Mode != cutover.ModeSwitch {
		writeError(w, http.StatusBadRequest, "mode must be dry_run or switch")
		return
	}
	if req.Mode == cutover.ModeSwitch && strings.TrimSpace(req.Confirm) != it.TargetName {
		writeError(w, http.StatusBadRequest, "type the app name "+it.TargetName+" to confirm the switch")
		return
	}
	if it.State != store.AppImportVerified && it.State != store.AppImportRouted {
		writeError(w, http.StatusConflict, "verify the app first, it is "+it.State)
		return
	}
	runner := rt.cutoverRunner()
	if runner == nil {
		writeError(w, http.StatusNotImplemented, "cutover runs are not available on this control plane")
		return
	}
	if req.Mode == cutover.ModeSwitch {
		if runs, err := runner.Store.List(r.Context(), it.TargetName, 1); err == nil && len(runs) > 0 && runs[0].Mode == cutover.ModeSwitch && runs[0].State == cutover.StateLive {
			writeError(w, http.StatusConflict, "this app is already live from run "+runs[0].ID+", roll it back first")
			return
		}
	}
	run, err := runner.Start(r.Context(), cutover.StartRequest{SessionID: sess.ID, SourceID: it.SourceID, App: it.TargetName, Mode: req.Mode,
		DNSWrite: rt.callerIsRoot(r), WasRouted: it.State == store.AppImportRouted, AcceptWarn: req.AcceptWarnings})
	if errors.Is(err, store.ErrCutoverRunActive) {
		writeError(w, http.StatusConflict, "a cutover run is already in progress for this app")
		return
	}
	if err != nil {
		rt.internalError(w, "api: cutover: start failed", err, slog.String("session_id", sess.ID), slog.String("app", it.TargetName))
		return
	}
	rt.setAppImportStep(r.Context(), sess, appImportStepCutover)
	rt.launchCutover(runner, run.ID)
	writeJSON(w, http.StatusAccepted, toCutoverRunResource(run))
}

func (rt *Router) loadCutoverRun(w http.ResponseWriter, r *http.Request, it store.AppImportItem) (*cutover.Runner, cutover.Run, bool) {
	runner := rt.cutoverRunner()
	if runner == nil {
		writeError(w, http.StatusNotImplemented, "cutover runs are not available on this control plane")
		return nil, cutover.Run{}, false
	}
	run, err := runner.Store.Get(r.Context(), r.PathValue("run"))
	if err != nil || run.App != it.TargetName {
		writeError(w, http.StatusNotFound, "cutover run not found")
		return nil, cutover.Run{}, false
	}
	return runner, run, true
}

// handleListAppImportCutoverRuns handles GET .../items/{item}/cutover/runs.
func (rt *Router) handleListAppImportCutoverRuns(w http.ResponseWriter, r *http.Request) {
	_, it, ok := rt.loadCutoverItem(w, r)
	if !ok {
		return
	}
	runner := rt.cutoverRunner()
	if runner == nil {
		writeJSON(w, http.StatusOK, cutoverRunsResponse{Runs: []cutoverRunResource{}})
		return
	}
	runs, err := runner.Store.List(r.Context(), it.TargetName, cutoverRunListLimit)
	if err != nil {
		rt.internalError(w, "api: cutover: list runs failed", err, slog.String("app", it.TargetName))
		return
	}
	out := cutoverRunsResponse{Runs: make([]cutoverRunResource, 0, len(runs))}
	for _, run := range runs {
		out.Runs = append(out.Runs, toCutoverRunResource(run))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetAppImportCutoverRun handles GET .../cutover/runs/{run}.
func (rt *Router) handleGetAppImportCutoverRun(w http.ResponseWriter, r *http.Request) {
	_, it, ok := rt.loadCutoverItem(w, r)
	if !ok {
		return
	}
	_, run, ok := rt.loadCutoverRun(w, r, it)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toCutoverRunResource(run))
}

// handleRollbackAppImportCutover handles POST .../cutover/runs/{run}/rollback.
func (rt *Router) handleRollbackAppImportCutover(w http.ResponseWriter, r *http.Request) {
	_, it, ok := rt.loadCutoverItem(w, r)
	if !ok {
		return
	}
	runner, run, ok := rt.loadCutoverRun(w, r, it)
	if !ok {
		return
	}
	rt.stopCutoverWorker(run.ID)
	got, err := runner.Rollback(context.WithoutCancel(r.Context()), run.ID, "rolled back by the operator")
	if err != nil && !got.Rollbackable() && got.State != cutover.StateFailed {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		rt.logger.Warn("api: cutover: rollback incomplete", slog.String("error", err.Error()), slog.String("run_id", run.ID))
	}
	rt.nudgeReconciler()
	writeJSON(w, http.StatusOK, toCutoverRunResource(got))
}

// handleConfirmAppImportCutoverDNS handles POST .../cutover/runs/{run}/confirm-dns:
// the operator changed the record by hand.
func (rt *Router) handleConfirmAppImportCutoverDNS(w http.ResponseWriter, r *http.Request) {
	_, it, ok := rt.loadCutoverItem(w, r)
	if !ok {
		return
	}
	runner, run, ok := rt.loadCutoverRun(w, r, it)
	if !ok {
		return
	}
	run, err := runner.AcknowledgeManual(r.Context(), run.ID)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	rt.launchCutover(runner, run.ID)
	writeJSON(w, http.StatusAccepted, toCutoverRunResource(run))
}
