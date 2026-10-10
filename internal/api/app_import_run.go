package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	appImportReadyPoll       = 2 * time.Second
	defaultAppImportReadyFor = 5 * time.Minute
)

type appImportItemsRequest struct {
	Items []string `json:"items,omitempty"`
}

func decodeItemsRequest(w http.ResponseWriter, r *http.Request) (appImportItemsRequest, bool) {
	var req appImportItemsRequest
	if r.ContentLength == 0 {
		return req, true
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	return req, true
}

func wantedItem(ids []string, it store.AppImportItem) bool {
	if len(ids) == 0 {
		return true
	}
	return stringIn(ids, it.SourceID) || stringIn(ids, it.TargetName)
}

func (rt *Router) saveAppImportItem(ctx context.Context, it store.AppImportItem) {
	if err := rt.appImports.SaveAppImportItem(ctx, it, time.Now()); err != nil {
		rt.logger.Error("api: app import: save item failed", slog.String("error", err.Error()), slog.String("session_id", it.SessionID), slog.String("source_id", it.SourceID))
	}
}

func (rt *Router) setAppImportStep(ctx context.Context, sess store.AppImportSession, step string) {
	if importStepIndex(sess.Step) >= importStepIndex(step) {
		return
	}
	sess.Step = step
	if err := rt.appImports.UpdateAppImportSession(ctx, sess, time.Now()); err != nil {
		rt.logger.Warn("api: app import: record step failed", slog.String("error", err.Error()), slog.String("session_id", sess.ID))
	}
}

func importStepIndex(s string) int {
	for i, v := range appImportSteps {
		if v == s {
			return i
		}
	}
	return -1
}

func itemByPlan(items []store.AppImportItem, sourceID string) int {
	for i, it := range items {
		if it.SourceID == sourceID {
			return i
		}
	}
	return -1
}

// handleStageAppImport handles POST .../sessions/{id}/stage. Apps are
// created suspended, unrouted, with env and secrets imported. Items already
// staged are skipped, so a re-run resumes where a failure stopped.
func (rt *Router) handleStageAppImport(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	live := rt.appImportLive.get(sess.ID)
	if live == nil {
		writeError(w, http.StatusConflict, "the source is not connected: connect it again with its token before staging")
		return
	}
	calc, err := rt.calcAppImport(r.Context(), sess, items, live)
	if err != nil {
		rt.internalError(w, "api: app import: evaluate session failed", err, slog.String("session_id", sess.ID))
		return
	}
	if !calc.pre.CanStage {
		writeJSON(w, http.StatusUnprocessableEntity, rt.appImportView(r.Context(), sess, items, calc))
		return
	}
	stripBindMounts(rt, r, calc.plan)
	for _, a := range calc.plan.Apps {
		if len(a.Secrets) > 0 && rt.secrets == nil {
			writeError(w, http.StatusNotImplemented, "secrets are not configured on this control plane (no master key set)")
			return
		}
	}
	if !rt.appImportLive.begin(sess.ID) {
		writeError(w, http.StatusConflict, "this session is already running")
		return
	}
	defer rt.appImportLive.end(sess.ID)

	ctx := r.Context()
	applier := &importApplier{rt: rt, sessionID: sess.ID, connect: calc.connect}
	for _, planned := range calc.plan.Apps {
		i := itemByPlan(items, planned.SourceID)
		if i < 0 || !items[i].Selected || !stageable(items[i].State) {
			continue
		}
		warnings, err := applier.CreateApp(ctx, planned)
		it := &items[i]
		it.TargetName, it.Domains = planned.Name, planned.Domains
		if err != nil {
			it.State = store.AppImportStageFailed
			it.Reason = redactImportError(err.Error(), live.req.Token)
			rt.logger.Warn("api: app import: stage failed", slog.String("error", it.Reason), slog.String("session_id", sess.ID), slog.String("app", planned.Name))
		} else {
			it.State, it.CreatedTarget = store.AppImportStaged, true
			it.Reason = strings.Join(warnings, "; ")
			it.Volumes = stagedVolumes(*it, planned)
			it.EntryJSON = freshRecord(calc, it.EntryJSON, planned.SourceID)
			rt.logger.Info("api: app import: app staged", slog.String("session_id", sess.ID), slog.String("app", planned.Name), slog.String("source_id", planned.SourceID))
		}
		rt.saveAppImportItem(ctx, *it)
	}
	for _, rep := range calc.plan.Report.Items {
		i := itemByPlan(items, rep.SourceID)
		if i < 0 || rep.Kind != "app" || !items[i].Selected {
			continue
		}
		switch rep.Status {
		case platformimport.StatusAlready:
			if stageable(items[i].State) {
				items[i].State, items[i].TargetName = store.AppImportStaged, rep.Target
				items[i].Reason = "already imported by an earlier run, left as it is"
				rt.saveAppImportItem(ctx, items[i])
			}
		case platformimport.StatusSkipped:
			items[i].State, items[i].Reason = store.AppImportStageFailed, strings.Join(rep.Reasons, "; ")
			rt.saveAppImportItem(ctx, items[i])
		}
	}
	rt.setAppImportStep(ctx, sess, appImportStepStage)
	rt.nudgeReconciler()
	rt.writeAppImportView(w, r, http.StatusOK, sess, items)
}

func stageable(state string) bool {
	return state == store.AppImportPlanned || state == store.AppImportStageFailed || state == store.AppImportRolledBack
}

func redactImportError(msg, token string) string {
	if token != "" {
		msg = strings.ReplaceAll(msg, token, "[redacted]")
	}
	return msg
}

// stagedVolumes renames an item's volumes to the Docker volume names the
// staged app really uses, keeping each source name for the copy command.
func stagedVolumes(it store.AppImportItem, p platformimport.AppPlan) []store.AppImportVolume {
	prev := map[string]store.AppImportVolume{}
	for _, v := range it.Volumes {
		prev[v.ContainerPath] = v
	}
	out := make([]store.AppImportVolume, 0, len(p.Volumes))
	for _, v := range p.Volumes {
		old := prev[v.ContainerPath]
		nv := store.AppImportVolume{ContainerPath: v.ContainerPath, SourceName: old.SourceName, Copied: old.Copied, CopiedAt: old.CopiedAt}
		if v.HostPath != "" {
			nv.Name = v.HostPath
			nv.SourceName = v.HostPath
		} else {
			nv.Name = "app-" + p.Name + "-" + v.Name
			if nv.SourceName == "" {
				nv.SourceName = v.Name
			}
		}
		out = append(out, nv)
	}
	return out
}

// handleVerifyAppImport handles POST .../sessions/{id}/verify. It builds or
// pulls each staged app, waits for the app's own readiness check and records
// pass or fail per app. A failing app never stops the others.
func (rt *Router) handleVerifyAppImport(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	req, ok := decodeItemsRequest(w, r)
	if !ok {
		return
	}
	var todo []store.AppImportItem
	for _, it := range items {
		if it.Selected && wantedItem(req.Items, it) && verifiable(it.State) {
			todo = append(todo, it)
		}
	}
	if len(todo) == 0 {
		writeError(w, http.StatusConflict, "no staged app to verify: stage the import first")
		return
	}
	if !rt.appImportLive.begin(sess.ID) {
		writeError(w, http.StatusConflict, "this session is already running")
		return
	}
	for i := range todo {
		todo[i].State, todo[i].Reason = store.AppImportBuilding, ""
		rt.saveAppImportItem(r.Context(), todo[i])
	}
	rt.setAppImportStep(r.Context(), sess, appImportStepVerify)
	go rt.runAppImportVerify(sess, todo) //nolint:gosec // outlives the request on purpose: builds take minutes

	fresh, err := rt.appImports.ListAppImportItems(r.Context(), sess.ID)
	if err != nil {
		rt.internalError(w, "api: app import: load items failed", err, slog.String("session_id", sess.ID))
		return
	}
	rt.writeAppImportView(w, r, http.StatusAccepted, sess, fresh)
}

func verifiable(state string) bool {
	switch state {
	case store.AppImportStaged, store.AppImportVerifyFailed, store.AppImportVerified:
		return true
	}
	return false
}

func (rt *Router) runAppImportVerify(sess store.AppImportSession, todo []store.AppImportItem) {
	defer rt.appImportLive.end(sess.ID)
	timeout := envDuration(envAppImportReadyTimeout, defaultAppImportReadyFor)
	for _, it := range todo {
		ctx, cancel := context.WithTimeout(context.Background(), timeout+10*time.Minute)
		state, reason := rt.verifyAppImportItem(ctx, it, timeout)
		cancel()
		it.State, it.Reason = state, reason
		rt.saveAppImportItem(context.Background(), it)
		rt.logger.Info("api: app import: verify finished", slog.String("session_id", sess.ID), slog.String("app", it.TargetName), slog.String("state", state))
	}
}

// verifyAppImportItem starts the staged app, builds it when it is built
// from git, and waits for readiness. The app is stopped again afterwards so
// nothing keeps running against shared databases before cutover.
func (rt *Router) verifyAppImportItem(ctx context.Context, it store.AppImportItem, timeout time.Duration) (state, reason string) {
	name := it.TargetName
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if err != nil {
		return store.AppImportVerifyFailed, "the staged app no longer exists, stage the import again"
	}
	if err := rt.apps.UpdateServiceSuspended(ctx, name, false); err != nil {
		return store.AppImportVerifyFailed, "could not start the staged app: " + err.Error()
	}
	rt.nudgeReconciler()
	fail := func(msg string) (string, string) {
		if err := rt.apps.UpdateServiceSuspended(context.WithoutCancel(ctx), name, true); err != nil {
			rt.logger.Error("api: app import: stop failed app failed", slog.String("error", err.Error()), slog.String("app", name))
		}
		rt.nudgeReconciler()
		return store.AppImportVerifyFailed, msg
	}
	if rec := decodeAppImportRecord(it.EntryJSON); rec.Entry.Source == "git" {
		if rt.gitSources == nil {
			return fail("git builds are not configured on this control plane")
		}
		gs, err := rt.gitSources.GetGitSource(ctx, name)
		if err != nil {
			return fail("no repository is connected: set it in the app's source settings, then verify again")
		}
		status, msg := rt.deployFromGitSourceAs(ctx, name, *gs, gs.Branch, "", nil, store.DeployAttemptSourceManual)
		if status >= http.StatusBadRequest {
			return fail(fmt.Sprintf("the build failed (%s), open the app's deploy history for the log", strings.TrimSpace(firstMessageLine(msg))))
		}
	}
	deadline := time.Now().Add(timeout)
	detail := ""
	for {
		ready, d := rt.appReadiness(ctx, *svc)
		detail = d
		if ready {
			break
		}
		if time.Now().After(deadline) || ctx.Err() != nil {
			return fail("not ready within " + timeout.String() + ": " + detail)
		}
		select {
		case <-ctx.Done():
			return fail("stopped: " + ctx.Err().Error())
		case <-time.After(appImportReadyPoll):
		}
	}
	if err := rt.apps.UpdateServiceSuspended(ctx, name, true); err != nil {
		return store.AppImportVerified, "ready (" + detail + "); it is still running because stopping it failed: " + err.Error()
	}
	rt.nudgeReconciler()
	return store.AppImportVerified, "ready (" + detail + "), stopped again until cutover"
}

func firstMessageLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// handleRollbackAppImport handles POST .../sessions/{id}/rollback. It
// deletes only apps this session created, identified by the session label,
// and never anything on the source platform.
func (rt *Router) handleRollbackAppImport(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	req, ok := decodeItemsRequest(w, r)
	if !ok {
		return
	}
	if !rt.appImportLive.begin(sess.ID) {
		writeError(w, http.StatusConflict, "this session is already running")
		return
	}
	defer rt.appImportLive.end(sess.ID)
	ctx := r.Context()
	for i := range items {
		it := &items[i]
		if !wantedItem(req.Items, *it) || it.TargetName == "" || stageable(it.State) {
			continue
		}
		it.Reason = rt.rollbackAppImportItem(ctx, sess.ID, *it)
		if !strings.HasPrefix(it.Reason, "left untouched") {
			it.State = store.AppImportRolledBack
			it.CreatedTarget = false
		}
		rt.saveAppImportItem(ctx, *it)
	}
	rt.nudgeReconciler()
	rt.writeAppImportView(w, r, http.StatusOK, sess, items)
}

func (rt *Router) rollbackAppImportItem(ctx context.Context, sessionID string, it store.AppImportItem) string {
	name := it.TargetName
	svc, err := rt.apps.GetDesiredService(ctx, name)
	if errors.Is(err, store.ErrServiceNotFound) {
		return "the app was already gone"
	}
	if err != nil {
		return "left untouched: could not load the app: " + err.Error()
	}
	if svc.Labels[platformimport.LabelSession] != sessionID {
		return "left untouched: this app was not created by this import"
	}
	if _, err := rt.deleteApp(ctx, name); err != nil {
		return "left untouched: " + err.Error()
	}
	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(ctx, name); err != nil {
			rt.logger.Warn("api: app import: remove secrets failed", slog.String("error", err.Error()), slog.String("app", name))
		}
	}
	if rt.gitSources != nil {
		if err := rt.gitSources.DeleteGitSource(ctx, name); err != nil && !errors.Is(err, store.ErrGitSourceNotFound) {
			rt.logger.Warn("api: app import: remove git source failed", slog.String("error", err.Error()), slog.String("app", name))
		}
	}
	if rt.secrets != nil {
		if err := rt.secrets.DeleteAll(ctx, store.GitSourceSecretsKey(name)); err != nil {
			rt.logger.Warn("api: app import: remove git secrets failed", slog.String("error", err.Error()), slog.String("app", name))
		}
	}
	if len(it.Volumes) > 0 {
		return "the app was removed; its volumes were kept on disk, delete them by hand once you no longer need the data"
	}
	return "the app was removed"
}

// freshRecord stores the current inventory row and the rewrites applied to it.
func freshRecord(calc *appImportCalc, raw, sourceID string) string {
	rec := decodeAppImportRecord(raw)
	for _, e := range calc.inv.Entries() {
		if e.SourceID == sourceID {
			rec.Entry = e
		}
	}
	rec.Rewritten = calc.rewritten[sourceID]
	out, err := json.Marshal(rec)
	if err != nil {
		return raw
	}
	return string(out)
}
