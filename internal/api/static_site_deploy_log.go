package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploylog"
	"github.com/GLINCKER/levelrail/internal/pages"
	"github.com/GLINCKER/levelrail/internal/secretmask"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/telemetry"
)

// staticLogLine is one build log line tagged with the Pages phase it ran in.
type staticLogLine struct {
	Step   string `json:"step,omitempty"`
	Stream string `json:"stream"`
	Line   string `json:"line"`
	At     string `json:"at,omitempty"`
}

type staticDeployLogResponse struct {
	DeployID string          `json:"deploy_id"`
	Status   string          `json:"status"`
	Running  bool            `json:"running"`
	Lines    []staticLogLine `json:"lines"`
}

// staticSiteAttempt checks the site exists and the deploy belongs to it, writing the error itself.
func (rt *Router) staticSiteAttempt(w http.ResponseWriter, r *http.Request) (*store.DeployAttempt, bool) {
	getter, ok := rt.staticSites.(staticSiteGetter)
	if !ok || rt.deployAttempts == nil {
		writeError(w, http.StatusNotImplemented, "static site deploys are not available on this control plane")
		return nil, false
	}
	name := r.PathValue("name")
	site, err := getter.GetStaticSite(r.Context(), name)
	if err != nil && !errors.Is(err, store.ErrServiceNotFound) {
		rt.internalError(w, "api: static deploy log: lookup", err, slog.String("name", name))
		return nil, false
	}
	if site == nil {
		writeError(w, http.StatusNotFound, "static site not found")
		return nil, false
	}
	return rt.loadAppAttempt(w, r)
}

// handleStaticSiteDeployLog handles GET /api/v1/static-sites/{name}/deploys/{deployId}/log.
// It returns the stored (or in-progress) build log as JSON lines with the phase each ran in.
func (rt *Router) handleStaticSiteDeployLog(w http.ResponseWriter, r *http.Request) {
	attempt, ok := rt.staticSiteAttempt(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var entries []telemetry.DeployLogEntry
	running := attempt.FinishedAt == nil
	logMax, retention := int64(0), time.Duration(0)
	if rt.runtimeSettings != nil {
		logMax, retention = rt.runtimeSettings.PagesBuildLogLimits(ctx)
	}
	if !running && pages.LogExpired(*attempt.FinishedAt, time.Now(), retention) {
		writeError(w, http.StatusGone, "build log is past its retention")
		return
	}
	recorded := false
	if running && rt.deployRecorder != nil {
		lines, _, unsubscribe, found := rt.deployRecorder.Snapshot(attempt.ID)
		if found {
			unsubscribe()
			recorded = true
			for _, ev := range lines {
				entries = append(entries, telemetry.DeployLogEntry{Stream: ev.Stream, Timestamp: ev.At, Message: ev.Line})
			}
		}
	}
	if !recorded {
		if rt.deployLogStore == nil {
			writeError(w, http.StatusNotImplemented, "deploy log storage is not configured on this control plane")
			return
		}
		stored, err := rt.deployLogStore.QueryDeployLog(ctx, attempt.ID)
		if err != nil {
			rt.internalError(w, "api: static deploy log: query", err, slog.String("deploy_id", attempt.ID))
			return
		}
		entries = stored
	}
	entries = rt.mergeTraceIntoLog(ctx, attempt.ID, entries)

	phases := rt.phaseStarts(ctx, attempt.ID)
	out := staticDeployLogResponse{DeployID: attempt.ID, Status: attempt.Status, Running: running, Lines: make([]staticLogLine, 0, len(entries))}
	for _, e := range entries {
		out.Lines = append(out.Lines, staticLogLine{
			Step: phaseAt(phases, e.Timestamp), Stream: e.Stream,
			Line: secretmask.Mask(e.Message), At: eventAt(e.Timestamp),
		})
	}
	if kept, cut := pages.Window(out.Lines, func(l staticLogLine) int { return len(l.Line) }, logMax); cut {
		out.Lines = append([]staticLogLine{{Stream: "system", Line: pages.TruncatedNotice}}, kept...)
	}
	if tail, err := strconv.Atoi(r.URL.Query().Get("tail")); err == nil && tail > 0 && tail < len(out.Lines) {
		out.Lines = out.Lines[len(out.Lines)-tail:]
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePagesDeployLogs handles GET /api/v1/pages/{site}/deploys/{deployId}/logs, the Pages-named
// alias of the static site log with the same cap, retention and secret masking.
func (rt *Router) handlePagesDeployLogs(w http.ResponseWriter, r *http.Request) {
	r.SetPathValue("name", r.PathValue("site"))
	rt.handleStaticSiteDeployLog(w, r)
}

// handleStaticSiteDeployLogStream handles GET .../deploys/{deployId}/log/stream by reusing the
// app deploy log SSE stream, so live and replayed output mask secrets the same way.
func (rt *Router) handleStaticSiteDeployLogStream(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.staticSiteAttempt(w, r); !ok {
		return
	}
	rt.handleDeployLogStream(w, r)
}

type phaseStart struct {
	phase string
	at    time.Time
}

// phaseStarts lists the static phase rows by start time.
func (rt *Router) phaseStarts(ctx context.Context, attemptID string) []phaseStart {
	var out []phaseStart
	for _, row := range rt.allTraceRows(ctx, attemptID) {
		if row.Step != deploylog.StepStaticPhase || row.StartedAt.IsZero() {
			continue
		}
		out = append(out, phaseStart{phase: strings.TrimPrefix(row.Key, deploylog.StepStaticPhase+"."), at: row.StartedAt})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].at.Before(out[j].at) })
	return out
}

// phaseAt returns the latest phase that started at or before t, or "" before the first.
func phaseAt(phases []phaseStart, t time.Time) string {
	cur := ""
	for _, p := range phases {
		if p.at.After(t) {
			break
		}
		cur = p.phase
	}
	return cur
}

// pagesResourceFromPath names the site resource from the {site} path value, same scheme as apps.
func pagesResourceFromPath(r *http.Request) string {
	return "app:" + r.PathValue("site")
}
