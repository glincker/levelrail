package takeover

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Errors a caller maps to a status.
var (
	ErrBlocked         = errors.New("takeover: the preflight has blocking checks")
	ErrNotRollbackable = errors.New("takeover: nothing to roll back")
	ErrNotFound        = errors.New("takeover: run not found")
)

// Store persists runs.
type Store interface {
	Create(ctx context.Context, r Run) error
	Get(ctx context.Context, id string) (Run, error)
	Save(ctx context.Context, r Run) error
	List(ctx context.Context, limit int) ([]Run, error)
	ListByState(ctx context.Context, states ...string) ([]Run, error)
}

// Containers starts and stops the old proxy through the Docker Engine API.
type Containers interface {
	Stop(ctx context.Context, name string) error
	Start(ctx context.Context, name string) error
}

// TLSSettings is the part of the ingress settings a takeover flips.
type TLSSettings struct {
	TerminatedUpstream bool
	PublicHTTPSPort    int
}

// Settings reads and writes those settings.
type Settings interface {
	GetTLS(ctx context.Context) (TLSSettings, error)
	SetTLS(ctx context.Context, s TLSSettings) error
}

// Verifier requests one domain over real HTTPS through this server's ingress.
type Verifier interface {
	Verify(ctx context.Context, domain string) DomainCheck
}

// Deps is everything a Runner acts through.
type Deps struct {
	Containers Containers
	Settings   Settings
	Verifier   Verifier
	// Domains lists what verification must cover.
	Domains func(ctx context.Context) ([]string, error)
	// Restart asks the process to restart itself, false when it cannot.
	Restart func() bool
	// Audit records one entry; nil records nothing.
	Audit func(ctx context.Context, action, detail string)
}

// Config bounds a run. Zero values use the defaults.
type Config struct {
	Window         time.Duration
	VerifyInterval time.Duration
	StartAttempts  int
	StartWait      time.Duration
	Now            func() time.Time
}

func (c Config) window() time.Duration {
	if c.Window > 0 {
		return c.Window
	}
	return DefaultWindow
}

func (c Config) interval() time.Duration {
	if c.VerifyInterval > 0 {
		return c.VerifyInterval
	}
	return 10 * time.Second
}

func (c Config) attempts() int {
	if c.StartAttempts > 0 {
		return c.StartAttempts
	}
	return 6
}

func (c Config) wait() time.Duration {
	if c.StartWait > 0 {
		return c.StartWait
	}
	return 3 * time.Second
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

// Runner drives takeovers and their rollbacks.
type Runner struct {
	Store Store
	Deps  Deps
	Cfg   Config
	Log   *slog.Logger

	mu sync.Mutex
}

// StartRequest is what a run needs from its plan.
type StartRequest struct {
	Plan  Plan
	Actor string
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("tk-%d", time.Now().UnixNano())
	}
	return "tk-" + hex.EncodeToString(b)
}

func (r *Runner) log() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}

func (r *Runner) audit(ctx context.Context, action, detail string) {
	if r.Deps.Audit != nil {
		r.Deps.Audit(ctx, action, detail)
	}
}

func (run *Run) step(id, title, state, detail string, now time.Time) {
	for i := range run.Steps {
		if run.Steps[i].ID != id {
			continue
		}
		s := &run.Steps[i]
		if s.StartedAt.IsZero() {
			s.StartedAt = now
		}
		s.State, s.Detail = state, detail
		if state == StepDone || state == StepFailed || state == StepSkipped {
			s.FinishedAt = now
		}
		return
	}
	st := Step{ID: id, Title: title, State: state, Detail: detail, StartedAt: now}
	if state == StepDone || state == StepFailed || state == StepSkipped {
		st.FinishedAt = now
	}
	run.Steps = append(run.Steps, st)
}

func (r *Runner) save(ctx context.Context, run *Run) error {
	run.UpdatedAt = r.Cfg.now()
	return r.Store.Save(ctx, *run)
}

// Start runs the first half of a takeover and returns once the restart is
// requested. created is false when an earlier run is already in flight or
// finished, which makes a repeated request a no-op.
func (r *Runner) Start(ctx context.Context, req StartRequest) (run Run, created bool, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if req.Plan.Verdict == VerdictBlocked {
		return Run{}, false, ErrBlocked
	}
	existing, err := r.Store.ListByState(ctx, StatePreparing, StateApplying, StateVerifying, StateRollingBack, StateDone)
	if err != nil {
		return Run{}, false, fmt.Errorf("takeover: list runs: %w", err)
	}
	if len(existing) > 0 {
		return existing[len(existing)-1], false, nil
	}
	if req.Plan.Verdict == VerdictDone {
		return Run{}, false, errors.New("takeover: this server already listens on the target ports")
	}
	tls, err := r.Deps.Settings.GetTLS(ctx)
	if err != nil {
		return Run{}, false, fmt.Errorf("takeover: read tls settings: %w", err)
	}
	now := r.Cfg.now()
	run = Run{ID: newID(), State: StatePreparing, Actor: req.Actor, TargetHTTP: req.Plan.TargetHTTP, TargetHTTPS: req.Plan.TargetHTTPS,
		WindowSeconds: int(r.Cfg.window().Seconds()), Steps: []Step{}, Verify: []DomainCheck{}, CreatedAt: now, UpdatedAt: now,
		Previous: Previous{HTTPAddr: req.Plan.CurrentHTTP, HTTPSAddr: req.Plan.CurrentHTTPS,
			TLSTerminatedUpstream: tls.TerminatedUpstream, PublicHTTPSPort: tls.PublicHTTPSPort}}
	for _, n := range req.Plan.ProxyNames {
		run.Previous.Proxies = append(run.Previous.Proxies, Proxy{Name: n, WasRunning: true})
	}
	run.DeadlineAt = now.Add(r.Cfg.window())
	run.step(StepRollback, "Record the way back", StepDone, "previous addresses, TLS settings and containers saved", now)
	if err := r.Store.Create(ctx, run); err != nil {
		return Run{}, false, fmt.Errorf("takeover: record run: %w", err)
	}
	r.audit(ctx, "takeover.start", fmt.Sprintf("run %s: %s and %s from %s and %s", run.ID, run.TargetHTTP, run.TargetHTTPS, run.Previous.HTTPAddr, run.Previous.HTTPSAddr))
	if err := r.prepare(ctx, &run); err != nil {
		return run, true, err
	}
	run.State = StateApplying
	run.DeadlineAt = r.Cfg.now().Add(r.Cfg.window())
	run.step(StepListen, "Listen on "+run.TargetHTTP+" and "+run.TargetHTTPS, StepDone, "stored, applied when the control plane restarts", r.Cfg.now())
	run.step(StepRestart, "Restart the control plane", StepRunning, "restarting", r.Cfg.now())
	if err := r.save(ctx, &run); err != nil {
		return run, true, r.abort(ctx, &run, fmt.Errorf("save run: %w", err))
	}
	r.audit(ctx, "takeover.restart", "run "+run.ID)
	if r.Deps.Restart == nil || !r.Deps.Restart() {
		return run, true, r.abort(ctx, &run, errors.New("the control plane cannot restart itself"))
	}
	return run, true, nil
}

func (r *Runner) prepare(ctx context.Context, run *Run) error {
	run.step(StepStopProxy, "Stop the old proxy", StepRunning, "", r.Cfg.now())
	for i := range run.Previous.Proxies {
		p := &run.Previous.Proxies[i]
		if err := r.Deps.Containers.Stop(ctx, p.Name); err != nil {
			run.step(StepStopProxy, "", StepFailed, "could not stop "+p.Name+": "+err.Error(), r.Cfg.now())
			return r.abort(ctx, run, fmt.Errorf("stop %s: %w", p.Name, err))
		}
		p.Stopped = true
		if err := r.save(ctx, run); err != nil {
			return r.abort(ctx, run, fmt.Errorf("save run: %w", err))
		}
		r.audit(ctx, "takeover.stop_proxy", "run "+run.ID+": stopped "+p.Name)
	}
	detail := "no proxy container held the ports"
	if len(run.Previous.Proxies) > 0 {
		detail = "stopped, not removed"
	}
	run.step(StepStopProxy, "", StepDone, detail, r.Cfg.now())
	run.step(StepTLS, "Let this server terminate TLS", StepRunning, "", r.Cfg.now())
	run.Previous.SettingsChanged = true
	if err := r.save(ctx, run); err != nil {
		return r.abort(ctx, run, fmt.Errorf("save run: %w", err))
	}
	if err := r.Deps.Settings.SetTLS(ctx, TLSSettings{TerminatedUpstream: false, PublicHTTPSPort: 0}); err != nil {
		run.step(StepTLS, "", StepFailed, err.Error(), r.Cfg.now())
		return r.abort(ctx, run, fmt.Errorf("update tls settings: %w", err))
	}
	run.step(StepTLS, "", StepDone, "tls_terminated_upstream is off", r.Cfg.now())
	r.audit(ctx, "takeover.tls_settings", "run "+run.ID)
	return nil
}

// abort undoes a run that never reached the restart: the old proxy starts
// again and nothing was applied, so no restart is needed.
func (r *Runner) abort(ctx context.Context, run *Run, cause error) error {
	run.Error = cause.Error()
	run.step(StepRestore, "Roll back", StepRunning, "", r.Cfg.now())
	r.restoreSettings(ctx, run)
	failed := r.startProxies(ctx, run)
	now := r.Cfg.now()
	run.FinishedAt = now
	if failed != "" {
		run.State = StateFailed
		run.step(StepRestore, "", StepFailed, failed, now)
		run.Error += "; " + failed
	} else {
		run.State = StateRolledBack
		run.step(StepRestore, "", StepDone, "the old proxy is running again", now)
	}
	_ = r.save(ctx, run)
	r.audit(ctx, "takeover.abort", "run "+run.ID+": "+run.Error)
	return fmt.Errorf("takeover: %w", cause)
}

func (r *Runner) restoreSettings(ctx context.Context, run *Run) {
	if !run.Previous.SettingsChanged {
		return
	}
	err := r.Deps.Settings.SetTLS(ctx, TLSSettings{TerminatedUpstream: run.Previous.TLSTerminatedUpstream, PublicHTTPSPort: run.Previous.PublicHTTPSPort})
	if err != nil {
		r.log().Error("takeover: restore tls settings failed", slog.String("run_id", run.ID), slog.String("error", err.Error()))
		return
	}
	run.Previous.SettingsChanged = false
}

// startProxies starts every container the run stopped, retrying while the
// ports are still being released. It returns a message when one stayed down.
func (r *Runner) startProxies(ctx context.Context, run *Run) string {
	var down []string
	for i := range run.Previous.Proxies {
		p := &run.Previous.Proxies[i]
		if !p.Stopped || p.Restored {
			continue
		}
		var err error
		for attempt := 0; attempt < r.Cfg.attempts(); attempt++ {
			if err = r.Deps.Containers.Start(ctx, p.Name); err == nil {
				break
			}
			select {
			case <-ctx.Done():
				return "interrupted while starting " + p.Name
			case <-time.After(r.Cfg.wait()):
			}
		}
		if err != nil {
			down = append(down, p.Name+": "+err.Error())
			continue
		}
		p.Restored = true
		r.audit(ctx, "takeover.start_proxy", "run "+run.ID+": started "+p.Name)
	}
	if len(down) == 0 {
		return ""
	}
	return "start these by hand with docker start: " + strings.Join(down, "; ")
}

// Rollback undoes a takeover that is applying, verifying or done: it records
// the intent and restarts so the old addresses apply; Resume then starts the
// old proxy. Repeating it on a run already rolling back or rolled back is a no-op.
func (r *Runner) Rollback(ctx context.Context, id, reason, actor string) (Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	run, err := r.Store.Get(ctx, id)
	if err != nil {
		return Run{}, err
	}
	if !run.Rollbackable() {
		if run.State == StateRollingBack || run.State == StateRolledBack {
			return run, nil
		}
		return run, ErrNotRollbackable
	}
	return r.beginRollback(ctx, run, reason, actor, true)
}

func (r *Runner) beginRollback(ctx context.Context, run Run, reason, actor string, restart bool) (Run, error) {
	now := r.Cfg.now()
	run.State, run.Error = StateRollingBack, reason
	run.step(StepRestore, "Roll back", StepRunning, reason, now)
	if err := r.save(ctx, &run); err != nil {
		return run, fmt.Errorf("takeover: save run: %w", err)
	}
	r.audit(ctx, "takeover.rollback", "run "+run.ID+": "+reason+" (by "+actor+")")
	if !restart {
		return run, nil
	}
	if r.Deps.Restart == nil || !r.Deps.Restart() {
		return run, errors.New("takeover: the control plane cannot restart itself, restart it by hand to finish the rollback")
	}
	return run, nil
}

// Resume continues whatever a restart interrupted. Call it once after the
// router is built; it returns when the work is finished or ctx ends.
func (r *Runner) Resume(ctx context.Context) {
	runs, err := r.Store.ListByState(ctx, StatePreparing, StateApplying, StateVerifying, StateRollingBack)
	if err != nil {
		r.log().Error("takeover: resume failed", slog.String("error", err.Error()))
		return
	}
	for _, run := range runs {
		switch run.State {
		case StateApplying:
			r.verifyLoop(ctx, run)
		default:
			r.finishRollback(ctx, run)
		}
	}
}

func (r *Runner) finishRollback(ctx context.Context, run Run) {
	if run.State != StateRollingBack {
		reason := "the control plane stopped before the takeover finished"
		var err error
		if run, err = r.beginRollback(ctx, run, reason, "system", false); err != nil {
			r.log().Error("takeover: rollback record failed", slog.String("run_id", run.ID), slog.String("error", err.Error()))
			return
		}
	}
	r.restoreSettings(ctx, &run)
	failed := r.startProxies(ctx, &run)
	now := r.Cfg.now()
	run.FinishedAt = now
	if failed != "" {
		run.State = StateFailed
		run.Error += "; " + failed
		run.step(StepRestore, "", StepFailed, failed, now)
	} else {
		run.State = StateRolledBack
		run.step(StepRestore, "", StepDone, "listen addresses and TLS settings restored, the old proxy is running again", now)
	}
	if err := r.save(ctx, &run); err != nil {
		r.log().Error("takeover: save rollback failed", slog.String("run_id", run.ID), slog.String("error", err.Error()))
	}
	r.audit(ctx, "takeover.rolled_back", fmt.Sprintf("run %s: state %s: %s", run.ID, run.State, run.Error))
}

// saveIf saves run only while the stored state is still want, so a rollback
// the operator asked for in the meantime is never overwritten.
func (r *Runner) saveIf(ctx context.Context, run *Run, want string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, err := r.Store.Get(ctx, run.ID)
	if err != nil || cur.State != want {
		return false
	}
	return r.save(ctx, run) == nil
}

// verifyLoop checks every domain until all pass or the window ends, then
// either marks the run done or rolls it back by itself.
func (r *Runner) verifyLoop(ctx context.Context, run Run) {
	run.DeadlineAt = r.Cfg.now().Add(r.Cfg.window())
	run.step(StepRestart, "", StepDone, "the control plane restarted", r.Cfg.now())
	run.step(StepVerify, "Verify each domain over real HTTPS", StepRunning, "", r.Cfg.now())
	run.State = StateVerifying
	if !r.saveIf(ctx, &run, StateApplying) {
		return
	}
	for {
		checks, ok := r.checkAll(ctx)
		run.Verify = checks
		if ok {
			now := r.Cfg.now()
			run.State, run.FinishedAt = StateDone, now
			run.step(StepVerify, "", StepDone, fmt.Sprintf("%d domains verified", len(checks)), now)
			if r.saveIf(ctx, &run, StateVerifying) {
				r.audit(ctx, "takeover.done", "run "+run.ID)
			}
			return
		}
		if !r.saveIf(ctx, &run, StateVerifying) {
			return
		}
		if !r.Cfg.now().Before(run.DeadlineAt) {
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(r.Cfg.interval()):
		}
	}
	reason := "verification did not succeed within " + r.Cfg.window().String() + ": " + failing(run.Verify)
	run.step(StepVerify, "", StepFailed, reason, r.Cfg.now())
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, err := r.Store.Get(ctx, run.ID)
	if err != nil || cur.State != StateVerifying {
		return
	}
	run.State = StateVerifying
	if _, err := r.beginRollback(ctx, run, reason, "system", true); err != nil {
		r.log().Error("takeover: automatic rollback could not restart", slog.String("run_id", run.ID), slog.String("error", err.Error()))
	}
}

func (r *Runner) checkAll(ctx context.Context) ([]DomainCheck, bool) {
	domains, err := r.Deps.Domains(ctx)
	if err != nil {
		return []DomainCheck{{Detail: "list domains: " + err.Error()}}, false
	}
	out := make([]DomainCheck, 0, len(domains))
	all := true
	for _, d := range domains {
		c := r.Deps.Verifier.Verify(ctx, d)
		c.Domain = d
		if !c.OK {
			all = false
		}
		out = append(out, c)
	}
	return out, all
}

func failing(checks []DomainCheck) string {
	var parts []string
	for _, c := range checks {
		if !c.OK {
			parts = append(parts, strings.TrimSpace(c.Domain+" "+c.Detail))
		}
	}
	return strings.Join(parts, "; ")
}

// Get returns one run.
func (r *Runner) Get(ctx context.Context, id string) (Run, error) {
	return r.Store.Get(ctx, id)
}

// List returns runs newest first.
func (r *Runner) List(ctx context.Context, limit int) ([]Run, error) {
	return r.Store.List(ctx, limit)
}
