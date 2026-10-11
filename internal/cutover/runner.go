package cutover

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Config bounds each wait in a run.
type Config struct {
	HealthTimeout  time.Duration
	ProbeTimeout   time.Duration
	ProbeInterval  time.Duration
	VerifyTimeout  time.Duration
	VerifyInterval time.Duration
	// ResumeMaxAge is how old an interrupted run may be and still be resumed
	// forward after a restart. Older ones are rolled back instead.
	ResumeMaxAge time.Duration
}

// DefaultConfig is the production set of bounds.
func DefaultConfig() Config {
	return Config{
		HealthTimeout:  5 * time.Minute,
		ProbeTimeout:   time.Minute,
		ProbeInterval:  2 * time.Second,
		VerifyTimeout:  5 * time.Minute,
		VerifyInterval: 10 * time.Second,
		ResumeMaxAge:   30 * time.Minute,
	}
}

// StartRequest creates a run.
type StartRequest struct {
	SessionID  string
	SourceID   string
	App        string
	Mode       string
	DNSWrite   bool
	WasRouted  bool
	AcceptWarn bool
}

// Runner drives runs through the state machine. Every step is idempotent
// and persisted, so a run interrupted at any point resumes or rolls back.
type Runner struct {
	Store Store
	Deps  Deps
	Cfg   Config
	Log   *slog.Logger
	Now   func() time.Time
	Sleep func(ctx context.Context, d time.Duration) error
	// Hook runs after each persisted step. A non-nil error aborts Execute
	// with no cleanup, which is how tests simulate a process crash.
	Hook func(run Run, step string) error
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

func (r *Runner) sleep(ctx context.Context, d time.Duration) error {
	if r.Sleep != nil {
		return r.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (r *Runner) log() *slog.Logger {
	if r.Log != nil {
		return r.Log
	}
	return slog.Default()
}

// NewRunID returns a fresh run id.
func NewRunID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("cutover: run id: %w", err)
	}
	return "cut_" + hex.EncodeToString(b), nil
}

// Start validates req and persists a new run in the planning state.
func (r *Runner) Start(ctx context.Context, req StartRequest) (Run, error) {
	if req.Mode != ModeDryRun && req.Mode != ModeSwitch {
		return Run{}, fmt.Errorf("cutover: unknown mode %q", req.Mode)
	}
	if req.App == "" {
		return Run{}, errors.New("cutover: app is required")
	}
	id, err := NewRunID()
	if err != nil {
		return Run{}, err
	}
	now := r.now()
	run := Run{ID: id, SessionID: req.SessionID, SourceID: req.SourceID, App: req.App, Mode: req.Mode, State: StatePlanning,
		DNSWrite: req.DNSWrite, WasRouted: req.WasRouted, AcceptWarn: req.AcceptWarn, Domains: []DomainRun{}, Steps: []Step{},
		CreatedAt: now, UpdatedAt: now}
	if err := r.Store.Create(ctx, run); err != nil {
		return Run{}, fmt.Errorf("cutover: create run for %q: %w", req.App, err)
	}
	return run, nil
}

func (r *Runner) save(ctx context.Context, run *Run) error {
	run.UpdatedAt = r.now()
	if err := r.Store.Save(context.WithoutCancel(ctx), *run); err != nil {
		return fmt.Errorf("cutover: save run %q: %w", run.ID, err)
	}
	return nil
}

// step runs fn as one timed, persisted timeline entry.
func (r *Runner) step(ctx context.Context, run *Run, name, domain string, fn func() (string, error)) error {
	started := r.now()
	detail, err := fn()
	st := Step{Name: name, Domain: domain, Detail: detail, StartedAt: started, FinishedAt: r.now(), State: StepDone}
	st.DurationMS = st.FinishedAt.Sub(started).Milliseconds()
	if err != nil {
		st.State = StepFailed
		if st.Detail == "" {
			st.Detail = err.Error()
		}
	}
	run.Steps = append(run.Steps, st)
	if serr := r.save(ctx, run); serr != nil {
		return infraErr{serr}
	}
	if r.Hook != nil {
		if herr := r.Hook(*run, name); herr != nil {
			return infraErr{herr}
		}
	}
	return err
}

type infraErr struct{ err error }

func (e infraErr) Error() string { return e.err.Error() }
func (e infraErr) Unwrap() error { return e.err }

// isInfra reports whether err is a persistence failure or a test hook abort
// that must stop Execute instead of being treated as a step outcome.
func isInfra(err error) bool {
	var h infraErr
	return errors.As(err, &h)
}

// failStep routes a step failure: infrastructure errors and cancellation
// stop the run where it is (a later Execute resumes), anything else undoes
// what the run changed so far.
func (r *Runner) failStep(ctx context.Context, run *Run, err error, switched bool) error {
	if isInfra(err) {
		return err
	}
	if cerr := ctx.Err(); cerr != nil {
		return fmt.Errorf("cutover: run %q interrupted: %w", run.ID, cerr)
	}
	if switched {
		return r.rollback(ctx, run, err.Error())
	}
	return r.abort(ctx, run, err.Error())
}

// Execute drives run id forward until it reaches a resting state: ready
// (dry run done), live, rolled_back, failed, or paused awaiting a manual DNS
// change. It returns the run as last persisted.
func (r *Runner) Execute(ctx context.Context, id string) (Run, error) {
	run, err := r.Store.Get(ctx, id)
	if err != nil {
		return Run{}, fmt.Errorf("cutover: load run %q: %w", id, err)
	}
	for {
		if run.Terminal() || (run.State == StateSwitching && run.Awaiting != "") {
			return run, nil
		}
		if err := ctx.Err(); err != nil {
			return run, fmt.Errorf("cutover: run %q interrupted: %w", id, err)
		}
		var err error
		switch run.State {
		case StatePlanning:
			err = r.doPlanning(ctx, &run)
		case StateStarting:
			err = r.doStarting(ctx, &run)
		case StateVerifying:
			err = r.doVerifying(ctx, &run)
		case StateSwitching:
			err = r.doSwitching(ctx, &run)
		default:
			return run, fmt.Errorf("cutover: run %q is in unexpected state %q", id, run.State)
		}
		if err != nil {
			return run, err
		}
	}
}

func (r *Runner) finish(ctx context.Context, run *Run, state, msg string) error {
	run.State, run.Error, run.Awaiting = state, msg, ""
	run.FinishedAt = r.now()
	return r.save(ctx, run)
}

func (r *Runner) domainNames(run *Run) []string {
	out := make([]string, 0, len(run.Domains))
	for _, d := range run.Domains {
		out = append(out, d.Domain)
	}
	return out
}

func (r *Runner) doPlanning(ctx context.Context, run *Run) error {
	var plan Plan
	err := r.step(ctx, run, StepReadiness, "", func() (string, error) {
		p, err := r.Deps.Planner.Plan(ctx, PlanRequest{SessionID: run.SessionID, SourceID: run.SourceID, App: run.App, DNSWrite: run.DNSWrite})
		plan = p
		if err != nil {
			return "", fmt.Errorf("compute the readiness plan: %w", err)
		}
		return "verdict " + p.Verdict, nil
	})
	if err != nil {
		if isInfra(err) {
			return err
		}
		if cerr := ctx.Err(); cerr != nil {
			return fmt.Errorf("cutover: run %q interrupted: %w", run.ID, cerr)
		}
		return r.finish(ctx, run, StateFailed, err.Error())
	}
	run.Plan = &plan
	run.Domains = run.Domains[:0]
	methods := map[string]bool{}
	for _, d := range plan.Domains {
		run.Domains = append(run.Domains, DomainRun{Domain: d.Domain, Method: d.Method, Provider: d.Provider, Zone: d.Zone})
		methods[d.Method] = true
	}
	run.Method = overallMethod(methods)
	switch {
	case plan.Verdict == VerdictBlocked:
		return r.finish(ctx, run, StateFailed, "not ready: "+joinChecks(plan, StatusBlock))
	case plan.Verdict == VerdictWarn && run.Mode == ModeSwitch && !run.AcceptWarn:
		return r.finish(ctx, run, StateFailed, "warnings must be accepted before switching: "+joinChecks(plan, StatusWarn))
	}
	run.State = StateStarting
	return r.save(ctx, run)
}

func overallMethod(m map[string]bool) string {
	delete(m, MethodNone)
	switch len(m) {
	case 0:
		return MethodNone
	case 1:
		for k := range m {
			return k
		}
	}
	return MethodMixed
}

func joinChecks(p Plan, status string) string {
	var parts []string
	for _, c := range p.Checks {
		if c.Status == status {
			parts = append(parts, c.Detail)
		}
	}
	return strings.Join(parts, "; ")
}

// abort undoes a failure before any traffic changed: the app is stopped
// and detached again unless it was already serving before the run.
func (r *Runner) abort(ctx context.Context, run *Run, reason string) error {
	msg := reason
	if err := r.cleanup(ctx, run); err != nil {
		if isInfra(err) {
			return err
		}
		msg += "; cleanup failed: " + err.Error()
	}
	return r.finish(ctx, run, StateFailed, msg)
}

func (r *Runner) cleanup(ctx context.Context, run *Run) error {
	if run.WasRouted {
		return nil
	}
	cctx := context.WithoutCancel(ctx)
	return r.step(cctx, run, StepCleanup, "", func() (string, error) {
		if err := r.Deps.Host.Unroute(cctx, run.App, r.domainNames(run)); err != nil {
			return "", fmt.Errorf("stop the staged app: %w", err)
		}
		return "the staged app is stopped and detached again", nil
	})
}

func (r *Runner) doStarting(ctx context.Context, run *Run) error {
	err := r.step(ctx, run, StepRoute, "", func() (string, error) {
		if err := r.Deps.Host.Route(ctx, run.App, r.domainNames(run)); err != nil {
			return "", fmt.Errorf("start the app and attach its domains: %w", err)
		}
		return "domains attached and the app started", nil
	})
	if err != nil {
		return r.failStep(ctx, run, err, false)
	}
	run.State = StateVerifying
	return r.save(ctx, run)
}

func (r *Runner) doVerifying(ctx context.Context, run *Run) error {
	err := r.step(ctx, run, StepHealthy, "", func() (string, error) {
		ok, detail, err := r.Deps.Host.WaitHealthy(ctx, run.App, r.Cfg.HealthTimeout)
		if err != nil {
			return "", fmt.Errorf("wait for the app: %w", err)
		}
		if !ok {
			return "", fmt.Errorf("the app did not become healthy: %s", detail)
		}
		return detail, nil
	})
	if err != nil {
		return r.failStep(ctx, run, err, false)
	}
	path := "/"
	if run.Plan != nil && run.Plan.HealthPath != "" {
		path = run.Plan.HealthPath
	}
	for i := range run.Domains {
		d := &run.Domains[i]
		err := r.step(ctx, run, StepIngress, d.Domain, func() (string, error) { return r.probe(ctx, d, path) })
		if err != nil {
			return r.failStep(ctx, run, err, false)
		}
	}
	if run.Mode == ModeDryRun {
		return r.finishDryRun(ctx, run)
	}
	run.State = StateSwitching
	return r.save(ctx, run)
}

func (r *Runner) probe(ctx context.Context, d *DomainRun, path string) (string, error) {
	deadline := r.now().Add(r.Cfg.ProbeTimeout)
	var last ProbeResult
	for {
		res, err := r.Deps.Ingress.Probe(ctx, d.Domain, path)
		if err == nil {
			last = res
			d.ProbeStatus, d.ProbeDetail = res.Status, res.Detail
			if res.OK {
				return fmt.Sprintf("answered %d through this node's ingress", res.Status), nil
			}
		} else {
			last = ProbeResult{Detail: err.Error()}
			d.ProbeStatus, d.ProbeDetail = 0, err.Error()
		}
		if !r.now().Before(deadline) {
			return "", fmt.Errorf("%s did not answer through this node's ingress: %s", d.Domain, firstNonEmpty(last.Detail, "no answer"))
		}
		if err := r.sleep(ctx, r.Cfg.ProbeInterval); err != nil {
			return "", err
		}
	}
}

func (r *Runner) finishDryRun(ctx context.Context, run *Run) error {
	err := r.step(ctx, run, StepDNSPreview, "", func() (string, error) {
		var notes []string
		for i := range run.Domains {
			d := &run.Domains[i]
			d.Message = "dry run: nothing was changed"
			notes = append(notes, d.Domain+" via "+d.Method)
		}
		return "would switch " + strings.Join(notes, ", "), nil
	})
	if err != nil {
		return err
	}
	if err := r.cleanup(ctx, run); err != nil {
		if isInfra(err) {
			return err
		}
		return r.finish(ctx, run, StateFailed, "dry run passed but cleanup failed: "+err.Error())
	}
	return r.finish(ctx, run, StateReady, "")
}

func (r *Runner) planFor(run *Run, domain string) DomainPlan {
	if run.Plan != nil {
		for _, d := range run.Plan.Domains {
			if d.Domain == domain {
				return d
			}
		}
	}
	return DomainPlan{Domain: domain}
}

func (r *Runner) doSwitching(ctx context.Context, run *Run) error {
	manualPending := false
	for i := range run.Domains {
		d := &run.Domains[i]
		if d.Switched {
			continue
		}
		plan := r.planFor(run, d.Domain)
		switch d.Method {
		case MethodNone:
			d.Switched = true
			d.Message = "already resolves to this server"
		case MethodManual:
			d.Manual = plan.Desired
			manualPending = true
		case MethodDNS:
			if err := r.switchDNS(ctx, run, d, plan); err != nil {
				return r.failStep(ctx, run, err, true)
			}
		case MethodProxy:
			err := r.step(ctx, run, StepSwitch, d.Domain, func() (string, error) {
				if err := r.Deps.Proxy.WriteRoute(ctx, d.Domain); err != nil {
					return "", fmt.Errorf("write the proxy route for %s: %w", d.Domain, err)
				}
				d.Switched = true
				return "managed proxy route written", nil
			})
			if err != nil {
				return r.failStep(ctx, run, err, true)
			}
		default:
			return r.rollback(ctx, run, fmt.Sprintf("unknown switch method %q for %s", d.Method, d.Domain))
		}
	}
	if manualPending {
		run.Awaiting = AwaitingManualDNS
		return r.step(ctx, run, StepManualDNS, "", func() (string, error) {
			return "waiting for the DNS record to be changed by hand", nil
		})
	}
	return r.postVerify(ctx, run)
}

func (r *Runner) switchDNS(ctx context.Context, run *Run, d *DomainRun, plan DomainPlan) error {
	if len(d.Previous) == 0 {
		d.Previous = plan.Replace
	}
	if d.Applied == nil {
		d.Applied = plan.Desired
	}
	if err := r.save(ctx, run); err != nil {
		return err
	}
	return r.step(ctx, run, StepSwitch, d.Domain, func() (string, error) {
		res, err := r.Deps.DNS.Apply(ctx, d.Domain)
		if err != nil {
			return "", fmt.Errorf("change the DNS record for %s: %w", d.Domain, err)
		}
		if res.Applied != nil {
			d.Applied = res.Applied
		}
		d.Provider, d.Zone, d.Message = firstNonEmpty(res.Provider, d.Provider), firstNonEmpty(res.Zone, d.Zone), res.Message
		if len(res.Previous) > 0 {
			d.Previous = res.Previous
		}
		d.Switched = true
		return firstNonEmpty(res.Message, "DNS record changed, previous value stored for undo"), nil
	})
}

// AcknowledgeManual records that the operator changed the DNS record by
// hand and persists it, without continuing the run.
func (r *Runner) AcknowledgeManual(ctx context.Context, id string) (Run, error) {
	run, err := r.Store.Get(ctx, id)
	if err != nil {
		return Run{}, fmt.Errorf("cutover: load run %q: %w", id, err)
	}
	if run.State != StateSwitching || run.Awaiting != AwaitingManualDNS {
		return run, errors.New("cutover: this run is not waiting for a manual DNS change")
	}
	for i := range run.Domains {
		if run.Domains[i].Method == MethodManual {
			run.Domains[i].Switched = true
		}
	}
	run.Awaiting = ""
	if err := r.save(ctx, &run); err != nil {
		return run, err
	}
	return run, nil
}

// ConfirmManual acknowledges the manual change, then continues with the
// post-switch verification.
func (r *Runner) ConfirmManual(ctx context.Context, id string) (Run, error) {
	if _, err := r.AcknowledgeManual(ctx, id); err != nil {
		return Run{}, err
	}
	return r.Execute(ctx, id)
}

func (r *Runner) postVerify(ctx context.Context, run *Run) error {
	deadline := r.now().Add(r.Cfg.VerifyTimeout)
	var failing []string
	for {
		failing = failing[:0]
		for i := range run.Domains {
			d := &run.Domains[i]
			if d.Verified {
				continue
			}
			ok, detail, err := r.Deps.Verifier.Verify(ctx, run.App, d.Domain)
			d.VerifyDetail = detail
			if err != nil {
				d.VerifyDetail = err.Error()
			}
			if err == nil && ok {
				d.Verified = true
				continue
			}
			failing = append(failing, d.Domain+": "+d.VerifyDetail)
		}
		if len(failing) == 0 {
			break
		}
		if !r.now().Before(deadline) {
			r.recordStep(run, StepPostVerify, StepFailed, "", strings.Join(failing, "; "))
			if err := r.save(ctx, run); err != nil {
				return err
			}
			return r.rollback(ctx, run, "post-switch verification failed: "+strings.Join(failing, "; "))
		}
		if err := r.sleep(ctx, r.Cfg.VerifyInterval); err != nil {
			return fmt.Errorf("cutover: run %q interrupted: %w", run.ID, err)
		}
	}
	r.recordStep(run, StepPostVerify, StepDone, "", "every domain verified through its public name")
	return r.finish(ctx, run, StateLive, "")
}

func (r *Runner) recordStep(run *Run, name, state, domain, detail string) {
	now := r.now()
	run.Steps = append(run.Steps, Step{Name: name, State: state, Domain: domain, Detail: detail, StartedAt: now, FinishedAt: now})
}

// Rollback restores every domain this run switched, then stops the staged
// app. It is safe to call again after a partial failure.
func (r *Runner) Rollback(ctx context.Context, id, reason string) (Run, error) {
	run, err := r.Store.Get(ctx, id)
	if err != nil {
		return Run{}, fmt.Errorf("cutover: load run %q: %w", id, err)
	}
	if !run.Rollbackable() {
		return run, errors.New("cutover: this run has nothing to roll back")
	}
	if reason == "" {
		reason = "rolled back by the operator"
	}
	err = r.rollback(ctx, &run, reason)
	return run, err
}

func (r *Runner) rollback(ctx context.Context, run *Run, reason string) error {
	cctx := context.WithoutCancel(ctx)
	var problems []string
	for i := len(run.Domains) - 1; i >= 0; i-- {
		d := &run.Domains[i]
		if d.Restored || (!d.Switched && d.Applied == nil && len(d.Previous) == 0) {
			continue
		}
		err := r.step(cctx, run, StepRollback, d.Domain, func() (string, error) { return r.restoreDomain(cctx, d) })
		if err != nil {
			if isInfra(err) {
				return err
			}
			problems = append(problems, err.Error())
		}
	}
	if !run.WasRouted {
		err := r.step(cctx, run, StepCleanup, "", func() (string, error) {
			if err := r.Deps.Host.Unroute(cctx, run.App, r.domainNames(run)); err != nil {
				return "", fmt.Errorf("stop the staged app: %w", err)
			}
			return "the staged app is stopped and detached again", nil
		})
		if err != nil {
			if isInfra(err) {
				return err
			}
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		return r.finish(ctx, run, StateFailed, "rollback incomplete, run it again: "+strings.Join(problems, "; ")+" (reason: "+reason+")")
	}
	return r.finish(ctx, run, StateRolledBack, reason)
}

func (r *Runner) restoreDomain(ctx context.Context, d *DomainRun) (string, error) {
	switch d.Method {
	case MethodDNS:
		if err := r.Deps.DNS.Restore(ctx, d.Domain, d.Previous, d.Applied); err != nil {
			return "", fmt.Errorf("restore the DNS record for %s: %w", d.Domain, err)
		}
		d.Restored, d.Switched = true, false
		return "previous DNS value restored", nil
	case MethodProxy:
		if err := r.Deps.Proxy.RemoveRoute(ctx, d.Domain); err != nil {
			return "", fmt.Errorf("remove the proxy route for %s: %w", d.Domain, err)
		}
		d.Restored, d.Switched = true, false
		return "managed proxy route removed", nil
	default:
		d.Restored, d.Switched = true, false
		return "set the DNS record back by hand if you changed it", nil
	}
}

// Recover resumes or rolls back runs a restart interrupted. launch is
// called for each run to continue forward; stale ones are rolled back here.
func (r *Runner) Recover(ctx context.Context, launch func(id string)) error {
	runs, err := r.Store.ListByState(ctx, StatePlanning, StateStarting, StateVerifying, StateSwitching)
	if err != nil {
		return fmt.Errorf("cutover: list interrupted runs: %w", err)
	}
	for _, run := range runs {
		if run.Awaiting != "" {
			continue
		}
		if r.now().Sub(run.UpdatedAt) <= r.Cfg.ResumeMaxAge {
			launch(run.ID)
			continue
		}
		r.log().Warn("cutover: rolling back a stale interrupted run", slog.String("run_id", run.ID), slog.String("app", run.App), slog.String("state", run.State))
		run := run
		reason := "interrupted by a restart and too old to resume"
		var rerr error
		if run.Mode == ModeSwitch && (run.State == StateSwitching || hasSwitched(run)) {
			rerr = r.rollback(ctx, &run, reason)
		} else {
			rerr = r.abort(ctx, &run, reason)
		}
		if rerr != nil {
			r.log().Error("cutover: stale run cleanup failed", slog.String("run_id", run.ID), slog.String("error", rerr.Error()))
		}
	}
	return nil
}

func hasSwitched(run Run) bool {
	for _, d := range run.Domains {
		if d.Switched || d.Applied != nil {
			return true
		}
	}
	return false
}
