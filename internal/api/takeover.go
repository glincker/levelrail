package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/proxyroutes"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/takeover"
)

// Env knobs of the same-server port takeover.
const (
	envTakeoverWindow     = "APP_TAKEOVER_WINDOW"
	envTakeoverInterval   = "APP_TAKEOVER_VERIFY_INTERVAL"
	envTakeoverMinDiskGB  = "APP_TAKEOVER_MIN_DISK_GB"
	envTakeoverMinMemMB   = "APP_TAKEOVER_MIN_MEMORY_MB"
	takeoverRunListLimit  = 20
	takeoverStopTimeout   = 15 * time.Second
	takeoverDNSTimeout    = 5 * time.Second
	takeoverProbeTimeout  = 10 * time.Second
	defaultTakeoverMinGB  = 2
	defaultTakeoverMinMB  = 256
	takeoverAuditPathBase = "/api/v1/migration/takeover"
)

// takeoverService holds the test seams of the takeover routes.
type takeoverService struct {
	mu sync.Mutex
	// portListening and hostListeners are test seams for the host side.
	portListening func(port int) bool
	hostSS        func(ctx context.Context) (string, error)
	diskFreeGB    func() (float64, bool)
	memoryMB      func() int
	probe         func(ctx context.Context, t proxyroutes.ProbeTarget) proxyroutes.ProbeResult
	cfg           *takeover.Config
}

type takeoverRunResource struct {
	takeover.Run
	Rollbackable bool `json:"rollbackable"`
	InFlight     bool `json:"in_flight"`
}

func toTakeoverRunResource(r takeover.Run) takeoverRunResource {
	return takeoverRunResource{Run: r, Rollbackable: r.Rollbackable(), InFlight: r.InFlight()}
}

type takeoverStatusResponse struct {
	Plan takeover.Plan        `json:"plan"`
	Run  *takeoverRunResource `json:"run"`
}

type takeoverRunRequest struct {
	Confirm         bool `json:"confirm"`
	AcceptUnmanaged bool `json:"accept_unmanaged"`
}

type takeoverRunResponse struct {
	DryRun  bool                 `json:"dry_run"`
	Created bool                 `json:"created"`
	Plan    *takeover.Plan       `json:"plan,omitempty"`
	Run     *takeoverRunResource `json:"run,omitempty"`
	Error   string               `json:"error,omitempty"`
}

type takeoverRunsResponse struct {
	Runs []takeoverRunResource `json:"runs"`
}

type takeoverRollbackRequest struct {
	Reason string `json:"reason"`
}

func (rt *Router) takeoverConfig() takeover.Config {
	if rt.takeover.cfg != nil {
		return *rt.takeover.cfg
	}
	return takeover.Config{
		Window:         envDuration(envTakeoverWindow, takeover.DefaultWindow),
		VerifyInterval: envDuration(envTakeoverInterval, 0),
	}
}

func (rt *Router) takeoverRunner() *takeover.Runner {
	rows, ok := any(rt.appImports).(takeover.Rows)
	if !ok {
		return nil
	}
	return &takeover.Runner{Store: takeover.DBStore{DB: rows}, Cfg: rt.takeoverConfig(), Log: rt.logger, Deps: takeover.Deps{
		Containers: takeoverContainers{rt: rt},
		Settings:   takeoverSettings{rt: rt},
		Verifier:   takeoverVerifier{rt: rt},
		Domains:    rt.takeoverDomainNames,
		Restart:    rt.requestRestart,
		Audit:      rt.takeoverAudit,
	}}
}

// requestRestart asks the supervisor to relaunch the process, false when no
// supervisor would bring it back.
func (rt *Router) requestRestart() bool {
	s := &rt.restart
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.r.Supported() {
		return false
	}
	if !s.pending {
		s.pending = true
		time.AfterFunc(restartTriggerDelay, s.r.Trigger)
	}
	return true
}

func (rt *Router) takeoverAudit(ctx context.Context, action, detail string) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: takeover audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: "system", ActorName: "takeover", Ability: AbilityRoot, Method: action,
		Path: takeoverAuditPathBase + "#" + detail, StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: "system",
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: takeover audit save failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}

// ResumeTakeover finishes a takeover or rollback a restart interrupted. It
// blocks until that work ends, so call it in a goroutine after the router is
// built.
func (rt *Router) ResumeTakeover(ctx context.Context) {
	runner := rt.takeoverRunner()
	if runner == nil {
		return
	}
	runner.Resume(ctx)
}

type takeoverContainers struct{ rt *Router }

func (c takeoverContainers) runtime() (docker.Runtime, error) {
	if c.rt.execRuntime == nil {
		return nil, errors.New("docker is not available on this control plane")
	}
	return c.rt.execRuntime("")
}

func (c takeoverContainers) Stop(ctx context.Context, name string) error {
	rtm, err := c.runtime()
	if err != nil {
		return err
	}
	return rtm.Stop(ctx, name, takeoverStopTimeout)
}

func (c takeoverContainers) Start(ctx context.Context, name string) error {
	rtm, err := c.runtime()
	if err != nil {
		return err
	}
	return rtm.Start(ctx, name)
}

type takeoverSettings struct{ rt *Router }

func (s takeoverSettings) GetTLS(ctx context.Context) (takeover.TLSSettings, error) {
	cur, err := s.rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return takeover.TLSSettings{}, err
	}
	return takeover.TLSSettings{TerminatedUpstream: cur.TLSTerminatedUpstream, PublicHTTPSPort: cur.PublicHTTPSPort}, nil
}

func (s takeoverSettings) SetTLS(ctx context.Context, t takeover.TLSSettings) error {
	cur, err := s.rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		return err
	}
	cur.TLSTerminatedUpstream, cur.PublicHTTPSPort = t.TerminatedUpstream, t.PublicHTTPSPort
	if err := s.rt.ingressSettings.UpdateIngressSettings(ctx, cur); err != nil {
		return err
	}
	s.rt.nudgeReconciler()
	return nil
}

type takeoverVerifier struct{ rt *Router }

// Verify requests the domain over HTTPS on this server's own listener with the
// domain as SNI and Host, so DNS and any CDN in front cannot change the answer,
// and requires the route-check reply that only this ingress can give.
func (v takeoverVerifier) Verify(ctx context.Context, domain string) takeover.DomainCheck {
	rt := v.rt
	probe := proxyroutes.Probe
	if rt.takeover.probe != nil {
		probe = rt.takeover.probe
	}
	var marker *proxyroutes.Marker
	if rt.proxyIntegration != nil {
		marker = rt.proxyIntegration.marker
	}
	port := rt.ingressHTTPSPort()
	res := probe(ctx, proxyroutes.ProbeTarget{Domain: domain, Address: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), Path: "/", Timeout: takeoverProbeTimeout, Marker: marker})
	out := takeover.DomainCheck{Domain: domain, Status: res.StatusCode, Delivered: res.Delivered}
	switch {
	case !res.Reachable:
		out.Detail = orString(res.Err, "not reachable over HTTPS")
	case !res.CertTrusted:
		out.Detail = "the certificate is not trusted yet"
	case marker != nil && !res.Delivered:
		out.Detail = "something other than this server's ingress answered on the port"
	default:
		out.OK = true
	}
	return out
}

func orString(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func (rt *Router) localDockerRuntime() (docker.Runtime, bool) {
	if rt.execRuntime == nil {
		return nil, false
	}
	rtm, err := rt.execRuntime("")
	if err != nil || rtm == nil {
		return nil, false
	}
	return rtm, true
}

func (rt *Router) buildTakeoverPlan(ctx context.Context, accept bool) (takeover.Plan, error) {
	f, err := rt.takeoverFacts(ctx, accept)
	if err != nil {
		return takeover.Plan{}, err
	}
	return takeover.BuildPlan(f), nil
}

func (rt *Router) takeoverRunnerReady(w http.ResponseWriter) (*takeover.Runner, bool) {
	runner := rt.takeoverRunner()
	if runner == nil {
		writeError(w, http.StatusNotImplemented, "port takeover is not available on this control plane")
		return nil, false
	}
	return runner, true
}

// handleTakeoverPlan handles GET /api/v1/migration/takeover/plan. Read-only:
// it inspects containers, DNS and settings and changes nothing.
func (rt *Router) handleTakeoverPlan(w http.ResponseWriter, r *http.Request) {
	runner, ok := rt.takeoverRunnerReady(w)
	if !ok {
		return
	}
	plan, err := rt.buildTakeoverPlan(r.Context(), r.URL.Query().Get("accept_unmanaged") == "true")
	if err != nil {
		rt.internalError(w, "api: takeover plan", err)
		return
	}
	resp := takeoverStatusResponse{Plan: plan}
	if runs, err := runner.List(r.Context(), 1); err == nil && len(runs) > 0 {
		res := toTakeoverRunResource(runs[0])
		resp.Run = &res
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleStartTakeover handles POST /api/v1/migration/takeover/runs. Without
// confirm it is a dry run: the plan comes back and nothing changes.
func (rt *Router) handleStartTakeover(w http.ResponseWriter, r *http.Request) {
	runner, ok := rt.takeoverRunnerReady(w)
	if !ok {
		return
	}
	var req takeoverRunRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	plan, err := rt.buildTakeoverPlan(r.Context(), req.AcceptUnmanaged)
	if err != nil {
		rt.internalError(w, "api: takeover plan", err)
		return
	}
	if !req.Confirm {
		writeJSON(w, http.StatusOK, takeoverRunResponse{DryRun: true, Plan: &plan})
		return
	}
	_, _, actor, _ := rt.currentActor(r)
	run, created, err := runner.Start(r.Context(), takeover.StartRequest{Plan: plan, Actor: actor})
	switch {
	case errors.Is(err, takeover.ErrBlocked):
		writeJSON(w, http.StatusConflict, takeoverRunResponse{Plan: &plan, Error: "the preflight has blocking checks: fix them and re-check"})
	case err != nil && run.ID == "":
		writeJSON(w, http.StatusConflict, takeoverRunResponse{Plan: &plan, Error: err.Error()})
	case err != nil:
		res := toTakeoverRunResource(run)
		writeJSON(w, http.StatusConflict, takeoverRunResponse{Run: &res, Created: created, Error: err.Error()})
	default:
		res := toTakeoverRunResource(run)
		status := http.StatusAccepted
		if !created {
			status = http.StatusOK
		}
		writeJSON(w, status, takeoverRunResponse{Run: &res, Created: created})
	}
}

// handleListTakeoverRuns handles GET /api/v1/migration/takeover/runs.
func (rt *Router) handleListTakeoverRuns(w http.ResponseWriter, r *http.Request) {
	runner, ok := rt.takeoverRunnerReady(w)
	if !ok {
		return
	}
	runs, err := runner.List(r.Context(), takeoverRunListLimit)
	if err != nil {
		rt.internalError(w, "api: takeover runs", err)
		return
	}
	out := takeoverRunsResponse{Runs: make([]takeoverRunResource, 0, len(runs))}
	for _, run := range runs {
		out.Runs = append(out.Runs, toTakeoverRunResource(run))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetTakeoverRun handles GET /api/v1/migration/takeover/runs/{run}.
func (rt *Router) handleGetTakeoverRun(w http.ResponseWriter, r *http.Request) {
	runner, ok := rt.takeoverRunnerReady(w)
	if !ok {
		return
	}
	run, err := runner.Get(r.Context(), r.PathValue("run"))
	if errors.Is(err, takeover.ErrNotFound) {
		writeError(w, http.StatusNotFound, "takeover run not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get takeover run", err)
		return
	}
	writeJSON(w, http.StatusOK, toTakeoverRunResource(run))
}

// handleRollbackTakeover handles POST /api/v1/migration/takeover/runs/{run}/rollback.
func (rt *Router) handleRollbackTakeover(w http.ResponseWriter, r *http.Request) {
	runner, ok := rt.takeoverRunnerReady(w)
	if !ok {
		return
	}
	var req takeoverRollbackRequest
	if r.ContentLength != 0 {
		if err := decodeJSONBody(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	_, _, actor, _ := rt.currentActor(r)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "rolled back by " + actor
	}
	run, err := runner.Rollback(r.Context(), r.PathValue("run"), reason, actor)
	switch {
	case errors.Is(err, takeover.ErrNotFound):
		writeError(w, http.StatusNotFound, "takeover run not found")
	case errors.Is(err, takeover.ErrNotRollbackable):
		writeError(w, http.StatusConflict, "this takeover has nothing left to roll back")
	case err != nil:
		res := toTakeoverRunResource(run)
		writeJSON(w, http.StatusConflict, takeoverRunResponse{Run: &res, Error: err.Error()})
	default:
		writeJSON(w, http.StatusAccepted, takeoverRunResponse{Run: ptr(toTakeoverRunResource(run))})
	}
}

func ptr[T any](v T) *T { return &v }
