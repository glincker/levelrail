package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/kit/ids"
)

const (
	patchRunRunning   = "running"
	patchRunSucceeded = "succeeded"
	patchRunFailed    = "failed"
	patchRunTimedOut  = "timed_out"
	patchRemoteReason = "patch preview and apply run on the control plane's own host only; patch other nodes on the node itself for now"
)

type nodePatchPackage struct {
	Name             string `json:"name"`
	CurrentVersion   string `json:"current_version,omitempty"`
	NewVersion       string `json:"new_version"`
	Source           string `json:"source,omitempty"`
	Security         bool   `json:"security"`
	Kernel           bool   `json:"kernel"`
	ContainerRuntime bool   `json:"container_runtime"`
}

type nodePatchPreviewRequest struct {
	IncludeAll bool `json:"include_all"`
	Refresh    bool `json:"refresh"`
}

type nodePatchPreviewResponse struct {
	NodeID            string             `json:"node_id"`
	Manager           string             `json:"manager"`
	IncludeAll        bool               `json:"include_all"`
	Packages          []nodePatchPackage `json:"packages"`
	Held              []nodePatchPackage `json:"held"`
	OtherAvailable    int                `json:"other_available"`
	RebootRequiredNow bool               `json:"reboot_required_now"`
	RebootLikely      bool               `json:"reboot_likely"`
	PreviewID         string             `json:"preview_id"`
	ExpiresAt         time.Time          `json:"expires_at"`
	bin               string
}

type nodePatchApplyRequest struct {
	PreviewID        string `json:"preview_id"`
	IncludeAll       bool   `json:"include_all"`
	RebootIfRequired bool   `json:"reboot_if_required"`
	Confirm          bool   `json:"confirm"`
}

type nodePatchCounts struct {
	Total    int `json:"total"`
	Security int `json:"security"`
}

type nodePatchRun struct {
	ID               string           `json:"id"`
	NodeID           string           `json:"node_id"`
	Actor            string           `json:"actor"`
	Manager          string           `json:"manager"`
	IncludeAll       bool             `json:"include_all"`
	RebootIfRequired bool             `json:"reboot_if_required"`
	Status           string           `json:"status"`
	StartedAt        time.Time        `json:"started_at"`
	FinishedAt       *time.Time       `json:"finished_at,omitempty"`
	Packages         []string         `json:"packages"`
	Held             []string         `json:"held"`
	ExitCode         int              `json:"exit_code"`
	Error            string           `json:"error,omitempty"`
	OutputTail       string           `json:"output_tail"`
	RebootRequired   bool             `json:"reboot_required"`
	RebootScheduled  bool             `json:"reboot_scheduled"`
	RebootNote       string           `json:"reboot_note,omitempty"`
	PatchStatusAfter *nodePatchCounts `json:"patch_status_after,omitempty"`
}

type patchPreviewRecord struct {
	id          string
	fingerprint string
	includeAll  bool
	expires     time.Time
}

// nodePatcher holds previews, the per-node apply lock and recent runs in memory.
type nodePatcher struct {
	runner   hostPatchRunner
	mu       sync.Mutex
	previews map[string]patchPreviewRecord
	busy     map[string]bool
	runs     map[string][]nodePatchRun
	// live maps a running upgrade unit (or direct child) to its node; it outlives the request lock.
	live      map[string]string
	scanned   chan struct{} // closed once leftover units from a previous process are tracked
	scanDone  bool
	adoptOnce sync.Once
	done      chan string // tests wait on finished run ids; nil in production
	// timeout, pollEvery and adoptMax override the runtime settings in tests only.
	timeout, pollEvery time.Duration
	adoptMax           int
}

func newNodePatcher(r hostPatchRunner) *nodePatcher {
	return &nodePatcher{runner: r, previews: map[string]patchPreviewRecord{}, busy: map[string]bool{}, runs: map[string][]nodePatchRun{},
		live: map[string]string{}, scanned: make(chan struct{})}
}

func (rt *Router) buildPatcher() *nodePatcher {
	rt.nodePatchOnce.Do(func() {
		if rt.nodePatch == nil {
			rt.nodePatch = newNodePatcher(realHostPatchRunner{})
		}
	})
	return rt.nodePatch
}

func (rt *Router) patcher() *nodePatcher {
	p := rt.buildPatcher()
	rt.startNodePatchAdoption(context.Background(), p)
	return p
}

// patchNode resolves the path node; only the control plane's own host is patchable today.
func (rt *Router) patchNode(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if rt.isLocalGuardNode(id) {
		return rt.guardKey(id), true
	}
	if _, err := rt.nodes.GetNode(r.Context(), id); errors.Is(err, store.ErrNodeNotFound) {
		writeError(w, http.StatusNotFound, "node not found")
		return "", false
	} else if err != nil {
		rt.internalError(w, "api: node patch: look up node failed", err, slog.String("node_id", id))
		return "", false
	}
	writeCodedError(w, http.StatusNotImplemented, "patch_unsupported", patchRemoteReason)
	return "", false
}

func decodePatchBody(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<12)).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}

func (rt *Router) buildPatchPlan(ctx context.Context, p *nodePatcher, includeAll, refresh bool) (nodePatchPreviewResponse, string, error) {
	manager, bin, err := detectPatchManager(p.runner)
	if err != nil {
		return nodePatchPreviewResponse{}, "", err
	}
	all, err := simulatePatches(ctx, p.runner, manager, bin, refresh)
	if err != nil {
		return nodePatchPreviewResponse{}, "", err
	}
	apply, held, other := splitPatchPlan(all, includeAll)
	resp := nodePatchPreviewResponse{Manager: manager, IncludeAll: includeAll, Packages: apply, Held: held, OtherAvailable: other, bin: bin}
	resp.RebootRequiredNow = manager == patchManagerAPT && p.runner.FileExists(aptRebootMarkerPath)
	for _, pkg := range apply {
		resp.RebootLikely = resp.RebootLikely || pkg.Kernel
	}
	return resp, patchPlanFingerprint(manager, includeAll, apply, held), nil
}

// handleNodePatchPreview handles POST /api/v1/nodes/{id}/patch/preview: a no-change simulation plus a short-lived preview id.
func (rt *Router) handleNodePatchPreview(w http.ResponseWriter, r *http.Request) {
	id, ok := rt.patchNode(w, r)
	if !ok {
		return
	}
	var req nodePatchPreviewRequest
	if !decodePatchBody(w, r, &req) {
		return
	}
	p := rt.patcher()
	p.mu.Lock()
	busy := p.nodeBusyLocked(id)
	p.mu.Unlock()
	if busy && req.Refresh {
		writeCodedError(w, http.StatusConflict, "patch_in_progress", "a patch run is in progress on this node; refresh after it finishes")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), p.applyTimeout(r.Context(), rt))
	defer cancel()
	resp, fp, err := rt.buildPatchPlan(ctx, p, req.IncludeAll, req.Refresh)
	if errors.Is(err, errNoPatchManager) {
		writeCodedError(w, http.StatusNotImplemented, "patch_manager_unsupported", err.Error())
		return
	}
	if err != nil {
		rt.internalError(w, "api: node patch preview failed", err, slog.String("node_id", id))
		return
	}
	resp.NodeID, resp.PreviewID = id, ids.MustHex(16)
	resp.ExpiresAt = time.Now().Add(rt.runtimeSettings.Duration(r.Context(), runtimesettings.KeyNodesPatchPreviewTTL))
	p.mu.Lock()
	p.previews[id] = patchPreviewRecord{id: resp.PreviewID, fingerprint: fp, includeAll: req.IncludeAll, expires: resp.ExpiresAt}
	p.mu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}

// patchDeployBusy fails closed: an unreadable deploy list counts as busy.
func (rt *Router) patchDeployBusy(ctx context.Context) bool {
	rt.buildStartMu.Lock()
	starting := len(rt.startingDeploys)
	rt.buildStartMu.Unlock()
	if starting > 0 || rt.deploySafety == nil {
		return starting > 0
	}
	running, err := rt.deploySafety.ListRunningDeployAttempts(ctx)
	if err != nil {
		rt.logger.Error("api: node patch: list running deploys failed", slog.String("error", err.Error()))
		return true
	}
	return len(running) > 0
}

// patchWorkloadCount counts apps and databases a reboot would take down; unpinned ones run on the local host.
func (rt *Router) patchWorkloadCount(ctx context.Context, nodeID string) (int, error) {
	keys := []string{nodeID}
	if rt.isLocalGuardNode(nodeID) {
		keys = []string{"", rt.localNodeID}
	}
	n := 0
	for _, k := range keys {
		if rt.apps != nil {
			s, err := rt.apps.ListDesiredServicesByNode(ctx, k)
			if err != nil {
				return 0, fmt.Errorf("list services on node: %w", err)
			}
			n += len(s)
		}
		if rt.databases != nil {
			d, err := rt.databases.ListDesiredDatabasesByNode(ctx, k)
			if err != nil {
				return 0, fmt.Errorf("list databases on node: %w", err)
			}
			n += len(d)
		}
	}
	return n, nil
}

// handleNodePatchApply handles POST /api/v1/nodes/{id}/patch/apply: installs exactly the previewed list in the background.
func (rt *Router) handleNodePatchApply(w http.ResponseWriter, r *http.Request) {
	id, ok := rt.patchNode(w, r)
	if !ok {
		return
	}
	var req nodePatchApplyRequest
	if !decodePatchBody(w, r, &req) {
		return
	}
	if !req.Confirm || req.PreviewID == "" {
		writeError(w, http.StatusBadRequest, "preview_id and confirm are required: run a patch preview first, then apply it with confirm true")
		return
	}
	p := rt.patcher()
	if !p.waitScanned(r.Context(), rt) {
		writeCodedError(w, http.StatusConflict, "adoption_pending",
			"upgrade units left by a previous control plane process are still being checked; the node stays locked until that succeeds, try again shortly")
		return
	}
	p.mu.Lock()
	if p.nodeBusyLocked(id) {
		p.mu.Unlock()
		writeCodedError(w, http.StatusConflict, "patch_in_progress", "a patch run is already in progress on this node")
		return
	}
	p.busy[id] = true
	p.mu.Unlock()
	started := false
	defer func() {
		if !started {
			p.release(id)
		}
	}()

	if rt.patchDeployBusy(r.Context()) {
		writeCodedError(w, http.StatusConflict, "deploy_in_progress", "a deploy is in progress on this node; apply patches after it finishes")
		return
	}
	p.mu.Lock()
	rec, have := p.previews[id]
	p.mu.Unlock()
	if !have || rec.id != req.PreviewID || time.Now().After(rec.expires) {
		writeCodedError(w, http.StatusConflict, "preview_expired", "this preview is unknown or expired; run a new patch preview")
		return
	}
	if rec.includeAll != req.IncludeAll {
		writeCodedError(w, http.StatusConflict, "preview_mismatch", "include_all must match the preview; run a new patch preview")
		return
	}
	if req.RebootIfRequired {
		n, err := rt.patchWorkloadCount(r.Context(), id)
		if err != nil {
			rt.internalError(w, "api: node patch: count workloads failed", err, slog.String("node_id", id))
			return
		}
		if n > 0 {
			writeCodedError(w, http.StatusConflict, "drain_required", fmt.Sprintf(
				"%d apps or databases run on this node and would go down on reboot; drain the node first, or apply without reboot_if_required", n))
			return
		}
	}
	simCtx, simCancel := context.WithTimeout(r.Context(), p.applyTimeout(r.Context(), rt))
	plan, fp, err := rt.buildPatchPlan(simCtx, p, req.IncludeAll, false)
	simCancel()
	if err != nil {
		rt.internalError(w, "api: node patch: re-simulate failed", err, slog.String("node_id", id))
		return
	}
	p.mu.Lock()
	delete(p.previews, id)
	p.mu.Unlock()
	if fp != rec.fingerprint {
		writeCodedError(w, http.StatusConflict, "preview_stale", "the package list changed since the preview; run a new patch preview")
		return
	}
	if len(plan.Packages) == 0 {
		writeCodedError(w, http.StatusConflict, "nothing_to_apply", "the preview has no packages to install")
		return
	}
	if planTouchesRuntime(plan.Packages) && !p.runner.UnderSystemd() {
		writeCodedError(w, http.StatusConflict, "runtime_upgrade_needs_systemd",
			"kernel, docker and containerd upgrades need the control plane to run under systemd with systemd-run, so the upgrade survives a docker restart; apply without include_all")
		return
	}
	extras, err := rt.patchDependencyCheck(r.Context(), p, plan)
	if err != nil {
		writeCodedError(w, http.StatusConflict, "simulation_failed", err.Error())
		return
	}
	if len(extras) > 0 {
		writeCodedError(w, http.StatusConflict, "preview_mismatch", "the install would also change packages outside the preview: "+
			strings.Join(extras, ", ")+"; dependencies that are not security updates are only planned with include_all, so preview again with include_all, or upgrade them yourself")
		return
	}
	run := nodePatchRun{ID: ids.MustHex(8), NodeID: id, Actor: rt.actorForRequest(r), Manager: plan.Manager, IncludeAll: req.IncludeAll,
		RebootIfRequired: req.RebootIfRequired, Status: patchRunRunning, StartedAt: time.Now(),
		Packages: patchNames(plan.Packages), Held: patchNames(plan.Held)}
	p.record(run, rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyNodesPatchRunHistory))
	rt.auditNodePatch(r, run, "start")
	started = true
	actor := rt.accessActor(r)
	bg := context.WithoutCancel(r.Context())
	go func() {
		defer p.release(id)
		var late <-chan patchInstallResult
		run, late = rt.executePatchRun(bg, p, run, plan)
		rt.saveNodePatchAudit(bg, actor, "", "", run, "finish")
		if late != nil {
			go rt.settleLatePatchRun(bg, p, actor, run, late)
		}
		if p.done != nil {
			p.done <- run.ID
		}
	}()
	writeJSON(w, http.StatusAccepted, run)
}

func patchNames(pkgs []nodePatchPackage) []string {
	out := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		out = append(out, p.Name)
	}
	return out
}

func planTouchesRuntime(pkgs []nodePatchPackage) bool {
	for _, p := range pkgs {
		if p.Kernel || p.ContainerRuntime {
			return true
		}
	}
	return false
}

func (p *nodePatcher) applyTimeout(ctx context.Context, rt *Router) time.Duration {
	if p.timeout > 0 {
		return p.timeout
	}
	return rt.runtimeSettings.Duration(ctx, runtimesettings.KeyNodesPatchApplyTimeout)
}

const (
	minPatchPollInterval = 500 * time.Millisecond
	maxPatchPollInterval = time.Minute
	// patchUnitQueryPolls bounds each systemctl or journalctl call to this many poll intervals.
	patchUnitQueryPolls = 5
)

// pollInterval clamps the setting here too: its range check only runs on API writes, not env seeds.
func (p *nodePatcher) pollInterval(ctx context.Context, rt *Router) time.Duration {
	if p.pollEvery > 0 {
		return p.pollEvery
	}
	return min(max(rt.runtimeSettings.Duration(ctx, runtimesettings.KeyNodesPatchPollInterval), minPatchPollInterval), maxPatchPollInterval)
}

// unitQuery runs a short, bounded systemctl or journalctl call; a hung one must not hold the node lock.
func (p *nodePatcher) unitQuery(ctx context.Context, rt *Router, bin string, args ...string) ([]byte, error) {
	qctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), patchUnitQueryPolls*p.pollInterval(ctx, rt))
	defer cancel()
	out, code, err := p.runner.Run(qctx, nil, bin, args...)
	if err == nil && code != 0 {
		err = fmt.Errorf("%s exited with code %d", bin, code)
	}
	return out, err
}

type patchInstallResult struct {
	out      string
	exitCode int
	err      error
	timedOut bool
	note     string
	late     <-chan patchInstallResult // set on timeout: delivers the result once the upgrade really ends
}

// runPatchInstall never signals the upgrade: on timeout it stops waiting and the install keeps going.
func (rt *Router) runPatchInstall(ctx context.Context, p *nodePatcher, run nodePatchRun, plan nodePatchPreviewResponse) patchInstallResult {
	install := patchInstallArgv(plan.Manager, plan.bin, plan.Packages)
	bg := context.WithoutCancel(ctx)
	deadline := time.NewTimer(p.applyTimeout(ctx, rt))
	defer deadline.Stop()
	if !p.runner.UnderSystemd() {
		key := "direct-" + run.ID
		p.track(key, run.NodeID)
		ch := make(chan patchInstallResult, 1)
		go func() {
			out, code, err := p.runner.Run(bg, nil, install[0], install[1:]...)
			p.untrack(key)
			ch <- patchInstallResult{out: string(out), exitCode: code, err: err}
		}()
		select {
		case res := <-ch:
			return res
		case <-deadline.C:
			return patchInstallResult{timedOut: true, exitCode: -1, late: ch,
				note: "the upgrade is still running on the host; the node stays locked until it finishes"}
		}
	}
	unit := patchUnitName(rt.patchBrandShort(), run.ID)
	desc := patchUnitDescription(run.IncludeAll, run.Packages)
	if out, code, err := p.runner.Run(bg, nil, patchBinSystemdRun, patchSystemdRunArgv(unit, desc, install)...); err != nil || code != 0 {
		return patchInstallResult{out: string(out), exitCode: code, err: err, note: "could not start the upgrade unit " + unit}
	}
	p.track(unit, run.NodeID)
	if st, ok := p.waitPatchUnit(ctx, rt, unit, deadline.C); ok {
		return p.finishPatchUnit(ctx, rt, unit, st)
	}
	late := make(chan patchInstallResult, 1)
	go func() {
		st, _ := p.waitPatchUnit(bg, rt, unit, nil)
		late <- p.finishPatchUnit(bg, rt, unit, st)
	}()
	return patchInstallResult{timedOut: true, exitCode: -1, late: late,
		note: "the upgrade is still running as unit " + unit + "; the node stays locked until it finishes"}
}

func (rt *Router) classifyPatchRun(ctx context.Context, run *nodePatchRun, res patchInstallResult) {
	run.ExitCode = res.exitCode
	run.OutputTail = lastPatchLines(res.out, rt.runtimeSettings.Int(ctx, runtimesettings.KeyNodesPatchOutputTailLines))
	run.RebootNote = res.note
	switch {
	case res.timedOut:
		run.Status = patchRunTimedOut
	case res.err != nil:
		run.Status, run.Error = patchRunFailed, maskPatchOutput(res.err.Error())
	case res.exitCode != 0:
		run.Status, run.Error = patchRunFailed, fmt.Sprintf("the package manager exited with code %d", res.exitCode)
	default:
		run.Status = patchRunSucceeded
	}
}

func (rt *Router) executePatchRun(ctx context.Context, p *nodePatcher, run nodePatchRun, plan nodePatchPreviewResponse) (nodePatchRun, <-chan patchInstallResult) {
	res := rt.runPatchInstall(ctx, p, run, plan)
	rt.classifyPatchRun(ctx, &run, res)
	if run.Status == patchRunSucceeded {
		run.RebootNote = ""
		run.RebootRequired = plan.RebootLikely || (plan.Manager == patchManagerAPT && p.runner.FileExists(aptRebootMarkerPath))
		if run.RebootRequired {
			rt.maybeRebootAfterPatch(ctx, p, &run)
		}
		simCtx, cancel := context.WithTimeout(ctx, p.applyTimeout(ctx, rt))
		if after, _, aerr := rt.buildPatchPlan(simCtx, p, true, false); aerr == nil {
			counts := nodePatchCounts{Total: len(after.Packages)}
			for _, pkg := range after.Packages {
				if pkg.Security {
					counts.Security++
				}
			}
			run.PatchStatusAfter = &counts
		}
		cancel()
	}
	finished := time.Now()
	run.FinishedAt = &finished
	p.update(run)
	rt.logger.Info("api: node patch run finished", slog.String("node_id", run.NodeID), slog.String("run_id", run.ID),
		slog.String("status", run.Status), slog.Int("packages", len(run.Packages)), slog.Bool("reboot_scheduled", run.RebootScheduled))
	return run, res.late
}

// maybeRebootAfterPatch re-checks workloads and deploys right before rebooting, since time passed during the upgrade.
func (rt *Router) maybeRebootAfterPatch(ctx context.Context, p *nodePatcher, run *nodePatchRun) {
	if !run.RebootIfRequired {
		run.RebootNote = "a reboot is required to finish this update; drain the node, then apply again with reboot_if_required or reboot it yourself"
		return
	}
	n, err := rt.patchWorkloadCount(ctx, run.NodeID)
	if err != nil || n > 0 || rt.patchDeployBusy(ctx) {
		run.RebootNote = "reboot skipped: apps, databases or a deploy are now on this node; drain it first"
		return
	}
	if _, code, rerr := p.runner.Run(context.WithoutCancel(ctx), nil, patchBinShutdown, "-r", "+1"); rerr != nil || code != 0 {
		run.RebootNote = fmt.Sprintf("reboot request failed (exit %d)", code)
		return
	}
	run.RebootScheduled = true
	run.RebootNote = "reboot scheduled in one minute"
}

func (p *nodePatcher) release(id string) {
	p.mu.Lock()
	delete(p.busy, id)
	p.mu.Unlock()
}

func (p *nodePatcher) record(run nodePatchRun, limit int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	list := append([]nodePatchRun{run}, p.runs[run.NodeID]...)
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	p.runs[run.NodeID] = list
}

func (p *nodePatcher) update(run nodePatchRun) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, r := range p.runs[run.NodeID] {
		if r.ID == run.ID {
			p.runs[run.NodeID][i] = run
		}
	}
}

// handleListNodePatchRuns handles GET /api/v1/nodes/{id}/patch/runs, newest first.
func (rt *Router) handleListNodePatchRuns(w http.ResponseWriter, r *http.Request) {
	id, ok := rt.patchNode(w, r)
	if !ok {
		return
	}
	p := rt.patcher()
	p.mu.Lock()
	out := append([]nodePatchRun{}, p.runs[id]...)
	p.mu.Unlock()
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) auditNodePatch(r *http.Request, run nodePatchRun, phase string) {
	rt.saveNodePatchAudit(r.Context(), rt.accessActor(r), clientIP(r), clientKindFromUserAgent(r.Header.Get("User-Agent")), run, phase)
}

// saveNodePatchAudit is the durable run record: in-memory history resets on restart, the audit log does not.
func (rt *Router) saveNodePatchAudit(ctx context.Context, a accessActor, remote, clientKind string, run nodePatchRun, phase string) {
	if rt.auditLog == nil {
		return
	}
	entryID, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: node patch audit id failed", slog.String("error", err.Error()))
		return
	}
	detail := fmt.Sprintf("phase=%s,run=%s,status=%s,exit=%d,include_all=%t,reboot_if_required=%t,reboot_scheduled=%t,packages=%s,held=%s",
		phase, run.ID, run.Status, run.ExitCode, run.IncludeAll, run.RebootIfRequired, run.RebootScheduled,
		strings.Join(validPatchNames(run.Packages), " "), strings.Join(validPatchNames(run.Held), " "))
	entry := store.AuditEntry{
		ID: entryID, ActorType: a.kind, ActorID: a.id, ActorName: a.name,
		Ability: AbilityRoot, Method: http.MethodPost, StatusCode: http.StatusAccepted,
		Path:       "/api/v1/nodes/" + run.NodeID + "/patch/apply#" + detail,
		RemoteAddr: remote, CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKind,
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(ctx), entry); err != nil {
		rt.logger.Warn("api: node patch audit save failed", slog.String("error", err.Error()), slog.String("node_id", run.NodeID))
	}
}

func validPatchNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, n := range names {
		if patchPackageName.MatchString(n) {
			out = append(out, n)
		}
	}
	return out
}
