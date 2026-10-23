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

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/deploytimeline"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// timelineStore is the optional persistence behind frozen timelines;
// *store.DB satisfies it. Without it every view is assembled live.
type timelineStore interface {
	SaveDeployTimeline(ctx context.Context, row store.DeployTimelineRow) error
	GetDeployTimeline(ctx context.Context, attemptID string) (store.DeployTimelineRow, error)
	PruneDeployTimelines(ctx context.Context, cutoff time.Time) (int64, error)
}

// Stream message types on GET /api/v1/deployments/{id}/timeline/stream.
const (
	timelineMsgSnapshot = "snapshot"
	timelineMsgEvent    = "event"
	timelineMsgState    = "state"
	timelineMsgEnd      = "end"
)

type timelineMessage struct {
	Type     string                     `json:"type"`
	Timeline *deploytimeline.Timeline   `json:"timeline,omitempty"`
	Event    *deploytimeline.Event      `json:"event,omitempty"`
	Verdict  deploytimeline.Verdict     `json:"verdict,omitempty"`
	Live     bool                       `json:"live"`
	Frozen   bool                       `json:"frozen,omitempty"`
	Phases   []deploytimeline.PhaseSpan `json:"phases,omitempty"`
}

// timelineAttempt loads the attempt behind id and applies the caller's app
// visibility. ok false means a response was written.
func (rt *Router) timelineAttempt(w http.ResponseWriter, r *http.Request, id string) (store.DeployAttempt, bool) {
	attempt, err := rt.deployAttempts.GetDeployAttempt(r.Context(), id)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy not found")
		return store.DeployAttempt{}, false
	}
	if err != nil {
		rt.internalError(w, "api: deploy timeline: load attempt failed", err, slog.String("deploy_id", id))
		return store.DeployAttempt{}, false
	}
	canRead, _, err := rt.callerAppVisibility(r)
	if err != nil {
		rt.internalError(w, "api: deploy timeline: resolve caller", err)
		return store.DeployAttempt{}, false
	}
	if !canRead(attempt.ServiceName) {
		writeError(w, http.StatusNotFound, "deploy not found")
		return store.DeployAttempt{}, false
	}
	return *attempt, true
}

func (rt *Router) timelineLogs(ctx context.Context, a store.DeployAttempt, trace []store.DeployTraceStep) []deploytimeline.LogLine {
	if rt.deployRecorder != nil && a.FinishedAt == nil {
		lines, _, unsubscribe, ok := rt.deployRecorder.Snapshot(a.ID)
		unsubscribe()
		if ok {
			base := a.StartedAt
			for _, s := range trace {
				if s.Step == "building" {
					base = s.StartedAt
					break
				}
			}
			out := make([]deploytimeline.LogLine, 0, len(lines))
			for i, l := range lines {
				out = append(out, deploytimeline.LogLine{At: base.Add(time.Duration(i) * time.Millisecond), Stream: l.Stream, Message: l.Line})
			}
			return out
		}
	}
	if rt.deployLogStore == nil {
		return nil
	}
	entries, err := rt.deployLogStore.QueryDeployLog(ctx, a.ID)
	if err != nil {
		rt.logger.Warn("api: deploy timeline: query build log failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
		return nil
	}
	out := make([]deploytimeline.LogLine, 0, len(entries))
	for _, e := range entries {
		out = append(out, deploytimeline.LogLine{At: e.Timestamp, Stream: e.Stream, Message: e.Message})
	}
	return out
}

var timelineTriggerPaths = []string{"deploys", "builds", "promote", "rollback"}

// timelineActor finds who called the trigger endpoint around the attempt's
// start. The audit trail holds no deploy id, so this is a time-and-path
// match and stays empty when nothing fits.
func (rt *Router) timelineActor(ctx context.Context, a store.DeployAttempt, window time.Duration) (actor, kind string) {
	switch a.Source {
	case store.DeployAttemptSourceGitPush, store.DeployAttemptSourceWebhook, store.DeployAttemptSourceAutoRollback:
		return "", ""
	}
	if rt.auditLog == nil {
		return "", ""
	}
	prefix := "/api/v1/apps/" + store.AuditLikeLiteral(a.ServiceName) + "/"
	var like []string
	for _, p := range timelineTriggerPaths {
		like = append(like, prefix+p+"%")
	}
	before := a.StartedAt.Add(window)
	entries, err := rt.auditLog.ListAuditEntries(ctx, 20, &before, store.AuditEntryFilter{Method: http.MethodPost, PathLike: like})
	if err != nil {
		rt.logger.Warn("api: deploy timeline: audit lookup failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
		return "", ""
	}
	earliest := a.StartedAt.Add(-window)
	for _, e := range entries {
		ts, perr := time.Parse(time.RFC3339Nano, e.CreatedAt)
		if perr != nil || ts.Before(earliest) || e.StatusCode >= http.StatusBadRequest {
			continue
		}
		name := firstNonBlank(e.AgentName, e.ActorName, e.ActorID)
		return name, e.ClientKind
	}
	return "", ""
}

func firstNonBlank(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func requestWindow(ctx context.Context, q telemetry.MetricsQuerier, app string, from, to time.Time) deploytimeline.Window {
	w := deploytimeline.Window{From: from, To: to}
	if !to.After(from) {
		return w
	}
	points, _ := telemetry.QueryRequests(ctx, q, app, from, to, to.Sub(from)+time.Second)
	if len(points) == 0 {
		return w
	}
	p := points[0]
	w.HasTraffic, w.Requests, w.RatePerSec = true, p.Requests, p.RatePerSec
	w.ErrorRate5xx, w.P95Ms = p.ErrorRate5xx, p.P95Ms
	return w
}

// timelineImpact measures request health on both sides of cutover. It returns
// nil when metrics are not configured or neither side saw any traffic.
func (rt *Router) timelineImpact(ctx context.Context, app string, cutover time.Time, cfg deploytimeline.Config, now time.Time) *deploytimeline.Impact {
	if rt.telemetry == nil {
		return nil
	}
	end := cutover.Add(cfg.ImpactWindow)
	complete := !now.Before(end)
	if !complete {
		end = now
	}
	im := &deploytimeline.Impact{
		CutoverAt: cutover, WindowMS: cfg.ImpactWindow.Milliseconds(), Complete: complete,
		Before: requestWindow(ctx, rt.telemetry, app, cutover.Add(-cfg.ImpactWindow), cutover),
		After:  requestWindow(ctx, rt.telemetry, app, cutover, end),
	}
	if !im.Before.HasTraffic && !im.After.HasTraffic {
		return nil
	}
	return im
}

func (rt *Router) timelineInputs(ctx context.Context, a store.DeployAttempt, cfg deploytimeline.Config, now time.Time, impact *deploytimeline.Impact) deploytimeline.Inputs {
	in := deploytimeline.Inputs{Attempt: a, Now: now, Config: cfg, Impact: impact}
	in.Trace = rt.traceRows(ctx, a.ID)
	if rt.deployGate != nil {
		events, err := rt.deployGate.ListGateEvents(ctx, a.ID)
		if err != nil {
			rt.logger.Warn("api: deploy timeline: list gate events failed", slog.String("deploy_id", a.ID), slog.String("error", err.Error()))
		}
		in.Gate = events
		if _, eff, serr := rt.effectiveSafety(ctx, a.ServiceName); serr == nil {
			st := deploysafety.WatchStatusOf(events, eff, now)
			in.WatchOpen = st.State == deploysafety.WatchStateWatching && now.Before(st.EndsAt.Add(cfg.SettleIdle))
		}
	}
	if rt.probeAttempts != nil {
		if probes, err := rt.probeAttempts.ListProbeAttempts(ctx, a.ID); err == nil {
			in.Probes = probes
		}
	}
	if rt.hookRuns != nil {
		if hooks, err := rt.hookRuns.GetHookRuns(ctx, a.ServiceName); err == nil {
			in.Hooks = hooks
		}
	}
	in.Logs = rt.timelineLogs(ctx, a, in.Trace)
	in.Actor, in.ActorKind = rt.timelineActor(ctx, a, cfg.ActorWindow)
	return in
}

// freezeTimeline stores tl for later replay and prunes expired ones. The
// first freeze wins, so a replay never shifts.
func (rt *Router) freezeTimeline(ctx context.Context, ts timelineStore, tl deploytimeline.Timeline, cfg deploytimeline.Config) bool {
	tl.Live = false
	payload, err := json.Marshal(tl)
	if err != nil {
		rt.logger.Error("api: deploy timeline: marshal failed", slog.String("deploy_id", tl.DeployID), slog.String("error", err.Error()))
		return false
	}
	if err := ts.SaveDeployTimeline(ctx, store.DeployTimelineRow{AttemptID: tl.DeployID, ServiceName: tl.App, FrozenAt: time.Now(), Payload: payload}); err != nil {
		rt.logger.Warn("api: deploy timeline: freeze failed", slog.String("deploy_id", tl.DeployID), slog.String("error", err.Error()))
		return false
	}
	if _, err := ts.PruneDeployTimelines(ctx, time.Now().Add(-cfg.Retention)); err != nil {
		rt.logger.Warn("api: deploy timeline: prune failed", slog.String("error", err.Error()))
	}
	return true
}

func (rt *Router) frozenTimeline(ctx context.Context, id string, cfg deploytimeline.Config) (deploytimeline.Timeline, bool) {
	ts, ok := rt.deployAttempts.(timelineStore)
	if !ok {
		return deploytimeline.Timeline{}, false
	}
	row, err := ts.GetDeployTimeline(ctx, id)
	if err != nil {
		if !errors.Is(err, store.ErrDeployTimelineNotFound) {
			rt.logger.Warn("api: deploy timeline: read frozen failed", slog.String("deploy_id", id), slog.String("error", err.Error()))
		}
		return deploytimeline.Timeline{}, false
	}
	if time.Since(row.FrozenAt) > cfg.Retention {
		return deploytimeline.Timeline{}, false
	}
	var tl deploytimeline.Timeline
	if err := json.Unmarshal(row.Payload, &tl); err != nil {
		rt.logger.Warn("api: deploy timeline: decode frozen failed", slog.String("deploy_id", id), slog.String("error", err.Error()))
		return deploytimeline.Timeline{}, false
	}
	frozenAt := row.FrozenAt
	tl.Frozen, tl.FrozenAt, tl.Live = true, &frozenAt, false
	return tl, true
}

// buildTimeline returns the frozen replay when one exists, else assembles the
// timeline now and freezes it once nothing more can be added.
func (rt *Router) buildTimeline(ctx context.Context, a store.DeployAttempt) deploytimeline.Timeline {
	cfg := deploytimeline.ConfigFromEnv()
	if tl, ok := rt.frozenTimeline(ctx, a.ID, cfg); ok {
		return tl
	}
	now := time.Now().UTC()
	in := rt.timelineInputs(ctx, a, cfg, now, nil)
	if cut, ok := deploytimeline.CutoverAt(in.Gate, in.Trace); ok {
		in.Impact = rt.timelineImpact(ctx, a.ServiceName, cut, cfg, now)
	}
	tl := deploytimeline.Assemble(in)
	if ts, ok := rt.deployAttempts.(timelineStore); ok && deploytimeline.Settled(in) {
		if rt.freezeTimeline(ctx, ts, tl, cfg) {
			tl.Frozen = true
		}
	}
	return tl
}

// handleGetDeployTimeline handles GET /api/v1/deployments/{id}/timeline: the
// whole ordered story of one deploy as JSON.
func (rt *Router) handleGetDeployTimeline(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.timelineAttempt(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, rt.buildTimeline(r.Context(), a))
}

// handleCompareDeployTimelines handles
// GET /api/v1/deployments/{id}/timeline/compare?with={id}: two timelines with
// per-phase durations and event codes only one of them has.
func (rt *Router) handleCompareDeployTimelines(w http.ResponseWriter, r *http.Request) {
	other := strings.TrimSpace(r.URL.Query().Get("with"))
	if other == "" {
		writeError(w, http.StatusBadRequest, "with is required: the deploy id to compare against")
		return
	}
	a, ok := rt.timelineAttempt(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	b, ok := rt.timelineAttempt(w, r, other)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, deploytimeline.Compare(rt.buildTimeline(r.Context(), a), rt.buildTimeline(r.Context(), b)))
}

func timelineStreamResource(*http.Request) string { return "deployments" }

func writeTimelineMessage(w http.ResponseWriter, m timelineMessage) {
	data, err := json.Marshal(m)
	if err != nil {
		slog.Default().Error("api: marshal deploy timeline message failed", slog.String("error", err.Error()))
		return
	}
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

// handleDeployTimelineStream handles
// GET /api/v1/deployments/{id}/timeline/stream: SSE of the assembled timeline.
// It sends one snapshot, then only events whose content changed, a state
// message when the verdict or phases move, and ends once the deploy settles.
func (rt *Router) handleDeployTimelineStream(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.timelineAttempt(w, r, r.PathValue("id"))
	if !ok {
		return
	}
	ctx := r.Context()
	cfg := deploytimeline.ConfigFromEnv()
	if tl, found := rt.frozenTimeline(ctx, a.ID, cfg); found {
		flusher, ok := startSSE(w)
		if !ok {
			return
		}
		_, _ = fmt.Fprint(w, ": connected\n\n")
		writeTimelineMessage(w, timelineMessage{Type: timelineMsgSnapshot, Timeline: &tl, Verdict: tl.Verdict, Frozen: true})
		writeTimelineMessage(w, timelineMessage{Type: timelineMsgEnd, Verdict: tl.Verdict, Frozen: true})
		flusher.Flush()
		return
	}
	flusher, ok := rt.startLiveSSE(w, r)
	if !ok {
		return
	}
	_, _ = fmt.Fprint(w, ": connected\n\n")
	flusher.Flush()

	sent := map[string]string{}
	var lastState string
	first := true
	var settledImpact *deploytimeline.Impact
	tick := time.NewTicker(durationEnv(EnvTracePollInterval, time.Second))
	defer tick.Stop()
	heartbeat := time.NewTicker(deploymentStreamHeartbeat)
	defer heartbeat.Stop()
	for {
		now := time.Now().UTC()
		cur, err := rt.deployAttempts.GetDeployAttempt(ctx, a.ID)
		if err == nil && cur != nil {
			a = *cur
		}
		in := rt.timelineInputs(ctx, a, cfg, now, nil)
		if settledImpact != nil {
			in.Impact = settledImpact
		} else if cut, found := deploytimeline.CutoverAt(in.Gate, in.Trace); found {
			in.Impact = rt.timelineImpact(ctx, a.ServiceName, cut, cfg, now)
			if in.Impact != nil && in.Impact.Complete {
				settledImpact = in.Impact
			}
		}
		tl := deploytimeline.Assemble(in)
		settled := deploytimeline.Settled(in)

		if first {
			first = false
			snap := tl
			writeTimelineMessage(w, timelineMessage{Type: timelineMsgSnapshot, Timeline: &snap, Verdict: tl.Verdict, Live: tl.Live})
			for _, e := range tl.Events {
				sent[e.ID] = e.Signature()
			}
		} else {
			for i := range tl.Events {
				e := tl.Events[i]
				if sig := e.Signature(); sent[e.ID] != sig {
					sent[e.ID] = sig
					writeTimelineMessage(w, timelineMessage{Type: timelineMsgEvent, Event: &e, Live: tl.Live})
				}
			}
		}
		if state := fmt.Sprintf("%s|%t|%d", tl.Verdict, tl.Live, len(tl.Phases)); state != lastState {
			lastState = state
			writeTimelineMessage(w, timelineMessage{Type: timelineMsgState, Verdict: tl.Verdict, Live: tl.Live, Phases: tl.Phases})
		}
		flusher.Flush()

		if settled {
			frozen := false
			if ts, ok := rt.deployAttempts.(timelineStore); ok {
				frozen = rt.freezeTimeline(context.WithoutCancel(ctx), ts, tl, cfg)
			}
			writeTimelineMessage(w, timelineMessage{Type: timelineMsgEnd, Verdict: tl.Verdict, Frozen: frozen})
			flusher.Flush()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case <-tick.C:
		}
	}
}
