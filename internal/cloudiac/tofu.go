package cloudiac

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Extra failure classes of the OpenTofu engine.
const (
	ErrLockHeld      ErrorKind = "lock_held"
	ErrPolicyRefused ErrorKind = "policy_refused"
	ErrUnsupported   ErrorKind = "not_implemented"
	// ErrPlanRejected means the saved plan is missing, does not match the approved hash, expired or is stale.
	ErrPlanRejected ErrorKind = "plan_rejected"
	// ErrStateDiverged means apply ran but its state could not be written under the fence.
	ErrStateDiverged ErrorKind = "state_diverged"
	// ErrQuarantineHeld means the stack has unrecovered quarantined state, so no run may start.
	ErrQuarantineHeld ErrorKind = "state_quarantined"
)

// Sentinels callers match with errors.Is.
var (
	ErrStateLocked         = errors.New("cloudiac: cloud state is locked by another run")
	ErrNotImplemented      = errors.New("cloudiac: step is not implemented yet")
	ErrPlanMismatch        = errors.New("cloudiac: the saved plan does not match the approved plan hash")
	ErrPlanExpired         = errors.New("cloudiac: the saved plan is older than cloud.plan_max_age_minutes")
	ErrPlanStale           = errors.New("cloudiac: the state changed after the plan was made")
	ErrDestroyNotConfirmed = errors.New("cloudiac: destroy needs an explicit confirmation")
	ErrStateWriteRejected  = errors.New("cloudiac: state write was rejected, the cloud and the stored state may differ")
	// ErrStateQuarantined fails closed: a plan against stale state could recreate resources that exist.
	ErrStateQuarantined = errors.New("cloudiac: the stack has quarantined state that must be recovered first")
)

// StateQuarantine carries the id of the quarantine row an apply wrote; match it with errors.As.
type StateQuarantine struct{ ID int64 }

func (q *StateQuarantine) Error() string {
	return fmt.Sprintf("cloudiac: post apply state is held in cloud state quarantine %d", q.ID)
}

const (
	tofuStateFile  = "terraform.tfstate"
	planFile       = "plan.tfplan"
	backendFile    = "levelrail_backend.tf.json"
	credEnvName    = "LR_OCI_CREDENTIAL_DIR" //nolint:gosec // an env name, not a credential
	stderrTailSize = 2048
	sensitiveValue = "(sensitive)"
	unknownValue   = "(known after apply)"
)

// StateLocker is the fenced lease part of the store.
type StateLocker interface {
	AcquireCloudStateLock(ctx context.Context, account, stack, holder string, ttl time.Duration, now time.Time) (store.CloudStateLock, error)
	ReleaseCloudStateLock(ctx context.Context, lock store.CloudStateLock) error
}

// OracleResolver turns a credential handle into the materializable Oracle credential.
type OracleResolver interface {
	ResolveOracle(ctx context.Context, h CredentialHandle) (cloudconn.OracleCredential, error)
}

// BinaryEnsurer returns a verified OpenTofu binary path; *Fetcher implements it.
type BinaryEnsurer interface {
	Ensure(ctx context.Context) (string, error)
}

// TofuConfig holds the engine knobs. Funcs are read per run so runtime settings apply live.
type TofuConfig struct {
	Runtime   string
	Image     string
	User      string
	PluginDir string
	// WorkBase holds the private run dirs; prefer a tmpfs, since the plaintext state and plan
	// sit there for the run. A disk backed base is allowed but reported as a plan warning.
	WorkBase string
	// Live refreshes the sandbox knobs from runtime settings at the start of every run.
	Live    func(TofuConfig) TofuConfig
	Holder  string
	Network NetworkPolicy
	Limits  SandboxLimits
	Timeout func() time.Duration
	// PlanMaxAge caps how old a saved plan may be when it is applied; zero refuses every apply.
	PlanMaxAge func() time.Duration
	Policy     func() Policy
	Cost       func() CostPolicy
	Prices     PriceTable
	// ExistingSpend reports the account's current monthly micros; nil means zero.
	ExistingSpend func(ctx context.Context, account string) int64
}

// TofuDeps are the collaborators; any nil one makes Plan fail with ErrNotConfigured.
type TofuDeps struct {
	Runner Runner
	Binary BinaryEnsurer
	Locks  StateLocker
	State  *StateStore
	Creds  OracleResolver
}

// TofuEngine is the OpenTofu Engine. Apply and destroy only ever run a saved, approved plan.
type TofuEngine struct {
	cfg  TofuConfig
	deps TofuDeps
	now  func() time.Time
}

// NewTofuEngine builds the engine; missing pieces surface at run time, not here.
func NewTofuEngine(cfg TofuConfig, deps TofuDeps) *TofuEngine {
	return &TofuEngine{cfg: cfg, deps: deps, now: time.Now}
}

// PlanAttr is one top level attribute change; sensitive values are already masked.
type PlanAttr struct {
	Path      string
	Old, New  string
	Sensitive bool
}

// PlanChange is one managed resource change from the saved plan.
type PlanChange struct {
	Address string
	Type    string
	Action  string
	Attrs   []PlanAttr
}

// PlanResult is the typed plan with its cost verdict and the hash a reviewer approves.
type PlanResult struct {
	Changes  []PlanChange
	Cost     CostEstimate
	Warnings []string
	Hash     string
}

// Summary is a one line count that never carries values.
func (p PlanResult) Summary() string {
	n := map[string]int{}
	for _, c := range p.Changes {
		n[c.Action]++
	}
	return fmt.Sprintf("plan: %d to create, %d to update, %d to replace, %d to delete",
		n[ActionCreate], n[ActionUpdate], n[ActionReplace], n[ActionDelete])
}

// Plan implements Engine.
func (e *TofuEngine) Plan(ctx context.Context, spec ModuleSpec, cred CredentialHandle) (Result, error) {
	pr, err := e.PlanDetailed(ctx, spec, cred)
	if err != nil {
		return Result{}, err
	}
	res := Result{Step: StepPlan, PlanHash: pr.Hash, Log: strings.Join(append([]string{pr.Summary()}, pr.Warnings...), "\n")}
	for _, c := range pr.Changes {
		res.Changes = append(res.Changes, Change{Address: c.Address, Action: c.Action})
	}
	return res, nil
}

func stepErr(step Step, kind ErrorKind, msg string, err error) error {
	return &Error{Kind: kind, Step: step, Message: msg, Err: err}
}

func planErr(kind ErrorKind, msg string, err error) error { return stepErr(StepPlan, kind, msg, err) }

// session is one locked run: the lease, the loaded state, the private run dir and the credential.
type session struct {
	cfg       TofuConfig
	account   string
	stack     string
	lock      store.CloudStateLock
	state     []byte
	version   int64
	run       *RunDir
	sb        SandboxSpec
	sensitive []string
	redact    func(string) string
	warnings  []string
	closers   []func()
}

func (s *session) close() {
	for i := len(s.closers) - 1; i >= 0; i-- {
		s.closers[i]()
	}
}

// begin takes the lease, loads state, runs check (which must not start a step) and prepares the run dir.
func (e *TofuEngine) begin(ctx context.Context, step Step, spec ModuleSpec, cred CredentialHandle, check func(*session) error) (*session, error) {
	d := e.deps
	cfg := e.cfg
	if cfg.Live != nil {
		cfg = cfg.Live(cfg)
	}
	if err := cfg.missing(d); err != nil {
		return nil, err
	}
	s := &session{cfg: cfg, account: cred.ID, stack: spec.Name}
	if s.account == "" || s.stack == "" || len(spec.Files) == 0 {
		return nil, stepErr(step, ErrInvalidSpec, "account, stack and module files are required", nil)
	}
	holder := cfg.Holder
	if holder == "" {
		holder = "levelrail-" + string(step)
	}
	lock, err := d.Locks.AcquireCloudStateLock(ctx, s.account, s.stack, holder, cfg.Timeout()+time.Minute, e.now())
	if errors.Is(err, store.ErrCloudStateLocked) {
		return nil, stepErr(step, ErrLockHeld, "another run holds the state lock for this stack", ErrStateLocked)
	}
	if err != nil {
		return nil, stepErr(step, ErrInternal, "state lock: "+err.Error(), err)
	}
	s.lock = lock
	s.closers = append(s.closers, func() { _ = d.Locks.ReleaseCloudStateLock(context.WithoutCancel(ctx), lock) })
	fail := func(err error) (*session, error) { s.close(); return nil, err }

	held, err := d.State.Quarantined(ctx, s.account, s.stack)
	if err != nil {
		return fail(stepErr(step, ErrInternal, "check state quarantine: "+err.Error(), errors.Join(ErrStateQuarantined, err)))
	}
	if len(held) > 0 {
		ids := make([]string, 0, len(held))
		for _, q := range held {
			ids = append(ids, fmt.Sprint(q.ID))
		}
		return fail(stepErr(step, ErrQuarantineHeld, "this stack has quarantined state ("+strings.Join(ids, ", ")+
			"); recover it with levelrail stack cloud-state recover before the next plan or apply", ErrStateQuarantined))
	}
	if s.state, s.version, err = d.State.Load(ctx, s.account, s.stack); err != nil {
		return fail(stepErr(step, ErrInternal, "load state: "+err.Error(), err))
	}
	if check != nil {
		if err := check(s); err != nil {
			return fail(err)
		}
	}
	bin, err := d.Binary.Ensure(ctx)
	if err != nil {
		return fail(stepErr(step, ErrInternal, "OpenTofu binary is not available: "+err.Error(), err))
	}
	oc, err := d.Creds.ResolveOracle(ctx, cred)
	if err != nil {
		return fail(stepErr(step, ErrCredential, "credential could not be resolved", err))
	}
	run, err := NewRunDir(cfg.WorkBase, cfg.User)
	if err != nil {
		return fail(stepErr(step, ErrInternal, "run dir: "+err.Error(), err))
	}
	s.run = run
	s.closers = append(s.closers, run.Cleanup)
	if !run.OnTmpfs {
		s.warnings = append(s.warnings, "warning: the cloud run directory is not on tmpfs, so the plaintext state touched disk during the run")
	}
	if free, ferr := FreeBytes(run.Root); ferr == nil && free < int64(cfg.Limits.WorkTmpMB)<<20 {
		return fail(stepErr(step, ErrInternal, "not enough free space for the cloud run directory", ErrLowDisk))
	}
	if err := writeModule(run.ModuleDir(), run.StateDir(), spec.Files, s.state); err != nil {
		return fail(stepErr(step, ErrInvalidSpec, err.Error(), err))
	}
	credMount := SandboxCredentialDir + "/credential"
	files, err := cloudconn.MaterializeOracle(run.Root, credMount, oc)
	if err != nil {
		return fail(stepErr(step, ErrCredential, "credential could not be prepared", err))
	}
	s.closers = append(s.closers, func() { _ = files.Close() })
	s.sensitive = SensitiveValues(s.state)
	s.redact = func(x string) string {
		return RedactStepOutput(x, stderrTailSize, files.Redact, s.sensitive, s.state)
	}
	s.sb, err = BuildSandboxSpec(SandboxInput{
		Runtime: cfg.Runtime, Image: cfg.Image, User: cfg.User, Step: step,
		ModuleDir: run.ModuleDir(), RunDir: run.Shared(), TofuBinary: bin, PluginDir: cfg.PluginDir,
		Credential: CredentialFile{HostPath: files.Dir(), EnvName: credEnvName},
		Network:    cfg.Network, Limits: cfg.Limits,
		Env: map[string]string{"OCI_CONFIG_FILE": credMount + "/" + cloudconn.OracleConfigFileName},
	})
	if err != nil {
		return fail(stepErr(step, ErrInvalidSpec, err.Error(), err))
	}
	return s, nil
}

// runSteps runs each argv in the sandbox and returns the last output.
func (e *TofuEngine) runSteps(ctx context.Context, s *session, step Step, steps ...[]string) (RunOutput, error) {
	var last RunOutput
	for i, args := range steps {
		sb := s.sb
		sb.Args = args
		out, err := e.deps.Runner.Run(ctx, sb)
		if err != nil {
			if i == len(steps)-1 {
				s.sensitive = append(s.sensitive, SensitiveValues(out.Stdout)...)
			}
			return out, stepFailure(ctx, step, args[1], out, err, s.redact)
		}
		last = out
	}
	return last, nil
}

func (c TofuConfig) timeoutCtx(ctx context.Context, step Step) (context.Context, context.CancelFunc, error) {
	if c.Timeout == nil || c.Timeout() <= 0 {
		return nil, nil, stepErr(step, ErrInternal, "engine timeout is not set", ErrNotConfigured)
	}
	ctx, cancel := context.WithTimeout(ctx, c.Timeout())
	return ctx, cancel, nil
}

// PlanDetailed runs init, plan and show under the state lease, saves the sealed plan and returns it typed.
// The account is the credential handle id and the stack is the module name.
func (e *TofuEngine) PlanDetailed(ctx context.Context, spec ModuleSpec, cred CredentialHandle) (PlanResult, error) {
	return e.plan(ctx, StepPlan, spec, cred)
}

// PlanDestroy saves a destroy plan; only Destroy with an explicit confirmation can run it.
func (e *TofuEngine) PlanDestroy(ctx context.Context, spec ModuleSpec, cred CredentialHandle) (PlanResult, error) {
	return e.plan(ctx, StepDestroy, spec, cred)
}

func (e *TofuEngine) plan(ctx context.Context, step Step, spec ModuleSpec, cred CredentialHandle) (PlanResult, error) {
	ctx, cancel, err := e.cfg.timeoutCtx(ctx, StepPlan)
	if err != nil {
		return PlanResult{}, err
	}
	defer cancel()
	s, err := e.begin(ctx, step, spec, cred, nil)
	if err != nil {
		return PlanResult{}, err
	}
	defer s.close()
	chdir := "-chdir=" + SandboxModuleDir
	show, err := e.runSteps(ctx, s, StepPlan,
		[]string{chdir, "init", "-input=false", "-no-color"},
		s.sb.Args,
		[]string{chdir, "show", "-json", "-no-color", SandboxStateDir + "/" + planFile})
	if err != nil {
		return PlanResult{}, err
	}
	if show.Truncated {
		return PlanResult{}, planErr(ErrInternal, "plan output exceeds the output cap", nil)
	}
	changes, err := parsePlanJSON(show.Stdout)
	if err != nil {
		return PlanResult{}, planErr(ErrProvider, "plan output could not be read", err)
	}
	pr, err := e.judge(ctx, s.cfg, StepPlan, s.account, toResourceChanges(changes))
	pr.Changes, pr.Warnings = changes, s.warnings
	if err != nil {
		return pr, err
	}
	raw, err := os.ReadFile(filepath.Join(s.run.StateDir(), planFile))
	if err != nil {
		return pr, planErr(ErrProvider, "the plan step left no saved plan", err)
	}
	defer clear(raw)
	pr.Hash, err = e.deps.State.SavePlan(ctx, s.account, s.stack, SavedPlan{
		Plan: raw, Changes: toResourceChanges(changes), StateVersion: s.version, Destroy: step == StepDestroy})
	if err != nil {
		return pr, planErr(ErrInternal, "save plan: "+err.Error(), err)
	}
	return pr, nil
}

// ApplyRequest names the approved saved plan. The module must be the one the plan was made from.
type ApplyRequest struct {
	Spec     ModuleSpec
	Cred     CredentialHandle
	PlanHash string
	// ConfirmDestroy must be set to run a destroy plan.
	ConfirmDestroy bool
}

// Apply runs the saved plan whose hash the caller approved, never a fresh diff.
func (e *TofuEngine) Apply(ctx context.Context, req ApplyRequest) (Result, error) {
	return e.applySaved(ctx, StepApply, req)
}

// Destroy runs a saved destroy plan; it needs the explicit confirmation and passes the destroy cap.
func (e *TofuEngine) Destroy(ctx context.Context, req ApplyRequest) (Result, error) {
	if !req.ConfirmDestroy {
		return Result{}, stepErr(StepDestroy, ErrPolicyRefused, "destroy needs an explicit confirmation", ErrDestroyNotConfirmed)
	}
	return e.applySaved(ctx, StepDestroy, req)
}

func (e *TofuEngine) applySaved(ctx context.Context, step Step, req ApplyRequest) (Result, error) {
	if req.PlanHash == "" {
		return Result{}, stepErr(step, ErrPlanRejected, "an approved plan hash is required", ErrPlanMismatch)
	}
	ctx, cancel, err := e.cfg.timeoutCtx(ctx, step)
	if err != nil {
		return Result{}, err
	}
	defer cancel()
	var saved SavedPlan
	check := func(s *session) error {
		var err error
		saved, err = e.checkSaved(ctx, s, step, req.PlanHash)
		return err
	}
	// The apply container runs StepApply args against the saved plan file for both apply and destroy.
	s, err := e.begin(ctx, StepApply, req.Spec, req.Cred, check)
	if err != nil {
		var ee *Error
		if errors.As(err, &ee) {
			ee.Step = step
		}
		return Result{}, err
	}
	defer s.close()
	defer clear(saved.Plan)
	res := Result{Step: step, PlanHash: saved.Hash, StateVersion: s.version}
	for _, c := range saved.Changes {
		res.Changes = append(res.Changes, Change{Address: c.Address, Action: c.Action})
	}
	if err := os.WriteFile(filepath.Join(s.run.StateDir(), planFile), saved.Plan, 0o600); err != nil {
		return res, stepErr(step, ErrInternal, "stage plan: "+err.Error(), err)
	}
	// One shot: once a step may have touched the cloud, the plan can never run again.
	defer func() { _ = e.deps.State.DropPlan(context.WithoutCancel(ctx), s.account, s.stack, saved.Hash) }()
	_, runErr := e.runSteps(ctx, s, step, []string{"-chdir=" + SandboxModuleDir, "init", "-input=false", "-no-color"}, s.sb.Args)

	// Write back whatever state the run left, even after a failure, so partial progress is never lost.
	next, rerr := os.ReadFile(filepath.Join(s.run.StateDir(), tofuStateFile))
	if rerr == nil && len(next) > 0 && !bytes.Equal(next, s.state) {
		defer clear(next)
		v, serr := e.deps.State.Save(context.WithoutCancel(ctx), s.lock, s.version, next)
		if serr != nil {
			res.Diverged = true
			qerr := e.quarantine(ctx, s, step, next, serr, runErr)
			if q := (*StateQuarantine)(nil); errors.As(qerr, &q) {
				res.Log = fmt.Sprintf("%s: state quarantined as %d", step, q.ID)
			}
			return res, qerr
		}
		res.StateVersion = v
		res.Partial = runErr != nil
	}
	if runErr != nil {
		return res, runErr
	}
	res.Log = fmt.Sprintf("%s: %d changes applied, state version %d", step, len(res.Changes), res.StateVersion)
	return res, nil
}

// quarantine seals the state the apply left before reporting, so the real resources it names stay recoverable.
func (e *TofuEngine) quarantine(ctx context.Context, s *session, step Step, state []byte, serr, runErr error) error {
	reason := "store_error"
	switch {
	case errors.Is(serr, store.ErrCloudStateLockLost):
		reason = "fence_moved"
	case errors.Is(serr, store.ErrCloudStateConflict):
		reason = "version_conflict"
	case errors.Is(serr, ErrStateTooLarge):
		reason = "too_large"
	}
	if runErr != nil {
		reason += ",apply_failed"
	}
	id, qerr := e.deps.State.Quarantine(context.WithoutCancel(ctx), s.account, s.stack, s.lock.Fence, reason, state)
	if qerr != nil {
		msg := "apply ran but its state could neither be stored nor quarantined; the cloud holds resources the stored state does not, reconcile by hand"
		return stepErr(step, ErrStateDiverged, msg, errors.Join(ErrStateWriteRejected, serr, qerr))
	}
	msg := fmt.Sprintf("apply ran but its state could not be stored under the lock fence (%s); it is sealed in cloud state quarantine %d and the stack is blocked until it is recovered", reason, id)
	return stepErr(step, ErrStateDiverged, msg, errors.Join(ErrStateWriteRejected, serr, &StateQuarantine{ID: id}))
}

// checkSaved refuses before any step runs: wrong hash, wrong kind, expired, stale, or now against policy.
func (e *TofuEngine) checkSaved(ctx context.Context, s *session, step Step, hash string) (SavedPlan, error) {
	saved, err := e.deps.State.LoadPlan(ctx, s.account, s.stack)
	if errors.Is(err, ErrPlanNotFound) {
		return saved, stepErr(step, ErrPlanRejected, "no saved plan for this stack, run a plan first", err)
	}
	if err != nil {
		return saved, stepErr(step, ErrInternal, "load plan: "+err.Error(), err)
	}
	if subtle.ConstantTimeCompare([]byte(saved.Hash), []byte(hash)) != 1 {
		return saved, stepErr(step, ErrPlanRejected, "the saved plan does not match the approved plan hash", ErrPlanMismatch)
	}
	if saved.Destroy != (step == StepDestroy) {
		return saved, stepErr(step, ErrPlanRejected, "a destroy plan runs only through destroy, and destroy runs only a destroy plan", ErrPlanMismatch)
	}
	maxAge := time.Duration(0)
	if s.cfg.PlanMaxAge != nil {
		maxAge = s.cfg.PlanMaxAge()
	}
	if maxAge <= 0 || e.now().Sub(saved.CreatedAt) > maxAge {
		return saved, stepErr(step, ErrPlanRejected, "the saved plan expired, run a new plan", ErrPlanExpired)
	}
	if saved.StateVersion != s.version {
		return saved, stepErr(step, ErrPlanRejected, "the state changed after this plan was made, run a new plan", ErrPlanStale)
	}
	_, err = e.judge(ctx, s.cfg, step, s.account, saved.Changes)
	return saved, err
}

// missing names the first unset piece, so an operator knows which setting to fill.
func (c TofuConfig) missing(d TofuDeps) error {
	need := func(msg string) error { return planErr(ErrInternal, msg, ErrNotConfigured) }
	switch {
	case c.Image == "":
		return need("the cloud engine is not configured: set the runtime setting cloud.engine_image to a digest pinned image")
	case c.Network.Network == "":
		return need("the cloud engine is not configured: set cloud.engine_network and cloud.engine_deny_addresses")
	case c.Runtime == "":
		return need("the cloud engine is not configured: no docker or podman runtime was found")
	case d.Binary == nil:
		return need("the cloud engine is not configured: the OpenTofu binary source is missing")
	case d.Creds == nil:
		return need("the cloud engine is not configured: no cloud credential resolver")
	case d.State == nil:
		return need("the cloud engine is not configured: the cloud state store is unavailable")
	case d.Runner == nil || d.Locks == nil || c.Timeout == nil || c.Policy == nil || c.Cost == nil || c.Prices == nil || c.WorkBase == "":
		return need("the cloud engine is not fully configured")
	}
	return nil
}

func stepFailure(ctx context.Context, step Step, name string, out RunOutput, err error, redact func(string) string) error {
	if ctx.Err() != nil {
		return stepErr(step, ErrCanceled, name+" stopped: "+ctx.Err().Error(), ctx.Err())
	}
	// Redact the whole capped stream before tailing, so the cut cannot split a secret.
	full := out.Stderr
	if limit := maxOutputBytes(); int64(len(full)) > limit {
		full = full[int64(len(full))-limit:]
	}
	msg := name + " failed"
	if t := redact(string(full)); t != "" {
		msg += ": " + t
	}
	return stepErr(step, ErrProvider, msg, errors.New(redact(err.Error())))
}

func toResourceChanges(changes []PlanChange) []ResourceChange {
	rc := make([]ResourceChange, 0, len(changes))
	for _, c := range changes {
		rc = append(rc, ResourceChange{Address: c.Address, Type: c.Type, Action: c.Action})
	}
	return rc
}

// judge applies the change set policy and the cost limit before a plan is returned or applied.
func (e *TofuEngine) judge(ctx context.Context, cfg TofuConfig, step Step, account string, rc []ResourceChange) (PlanResult, error) {
	var existing int64
	if cfg.ExistingSpend != nil {
		existing = cfg.ExistingSpend(ctx, account)
	}
	est := cfg.Cost().Estimate(cfg.Prices, rc, existing)
	refusals := append(cfg.Policy().Check(rc), est.Refusals...)
	res := PlanResult{Cost: est}
	if len(refusals) > 0 {
		codes := make([]string, 0, len(refusals))
		for _, r := range refusals {
			codes = append(codes, r.Error())
		}
		return res, stepErr(step, ErrPolicyRefused, strings.Join(codes, "; "), RefusalsError(refusals))
	}
	return res, nil
}

// writeModule writes the rendered files with an engine owned local backend that keeps the
// plaintext state in the writable state dir; the caller removes both dirs.
func writeModule(dir, stateDir string, files map[string][]byte, state []byte) error {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	for name, body := range files {
		if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") || name == tofuStateFile || strings.HasPrefix(name, "levelrail_") {
			return fmt.Errorf("module file name %q is not allowed", name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			return err
		}
	}
	backend, err := json.Marshal(map[string]any{"terraform": map[string]any{"backend": map[string]any{
		"local": map[string]any{"path": SandboxStateDir + "/" + tofuStateFile}}}})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, backendFile), backend, 0o600); err != nil {
		return err
	}
	if len(state) > 0 {
		return os.WriteFile(filepath.Join(stateDir, tofuStateFile), state, 0o600)
	}
	return nil
}

type planJSON struct {
	ResourceChanges []struct {
		Address string `json:"address"`
		Mode    string `json:"mode"`
		Type    string `json:"type"`
		Change  struct {
			Actions         []string       `json:"actions"`
			Before          map[string]any `json:"before"`
			After           map[string]any `json:"after"`
			AfterUnknown    map[string]any `json:"after_unknown"`
			BeforeSensitive any            `json:"before_sensitive"`
			AfterSensitive  any            `json:"after_sensitive"`
		} `json:"change"`
	} `json:"resource_changes"`
}

// parsePlanJSON maps show -json output to typed changes; data sources and no-ops are dropped.
func parsePlanJSON(raw []byte) ([]PlanChange, error) {
	var p planJSON
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, errors.New("plan json is malformed")
	}
	var out []PlanChange
	for _, rc := range p.ResourceChanges {
		if rc.Mode == "data" {
			continue
		}
		action, err := planAction(rc.Change.Actions)
		if err != nil {
			return nil, err
		}
		if action == ActionNoop || action == "" {
			continue
		}
		bs, as := sensitiveKeys(rc.Change.BeforeSensitive), sensitiveKeys(rc.Change.AfterSensitive)
		keys := map[string]bool{}
		for k := range rc.Change.Before {
			keys[k] = true
		}
		for k := range rc.Change.After {
			keys[k] = true
		}
		for k := range rc.Change.AfterUnknown {
			keys[k] = true
		}
		var attrs []PlanAttr
		for k := range keys {
			old, oldOK := rc.Change.Before[k]
			nv, newOK := rc.Change.After[k]
			unknown, _ := rc.Change.AfterUnknown[k].(bool)
			a := PlanAttr{Path: k, Old: valueString(old, oldOK), New: valueString(nv, newOK)}
			if unknown {
				a.New = unknownValue
			}
			if a.Old == a.New {
				continue
			}
			if bs[k] || as[k] {
				a.Sensitive = true
				if a.Old != "" {
					a.Old = sensitiveValue
				}
				if a.New != "" && !unknown {
					a.New = sensitiveValue
				}
			}
			attrs = append(attrs, a)
		}
		sort.Slice(attrs, func(i, j int) bool { return attrs[i].Path < attrs[j].Path })
		out = append(out, PlanChange{Address: rc.Address, Type: rc.Type, Action: action, Attrs: attrs})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address < out[j].Address })
	return out, nil
}

func planAction(acts []string) (string, error) {
	switch strings.Join(acts, ",") {
	case "create":
		return ActionCreate, nil
	case "update":
		return ActionUpdate, nil
	case "delete":
		return ActionDelete, nil
	case "delete,create", "create,delete":
		return ActionReplace, nil
	case "no-op", "read", "":
		return ActionNoop, nil
	}
	return "", fmt.Errorf("unknown plan action %q", strings.Join(acts, ","))
}

// sensitiveKeys returns top level keys marked sensitive, wholly or in any nested part.
func sensitiveKeys(v any) map[string]bool {
	out := map[string]bool{}
	m, ok := v.(map[string]any)
	if !ok {
		return out
	}
	for k, s := range m {
		if markedSensitive(s) {
			out[k] = true
		}
	}
	return out
}

func markedSensitive(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case map[string]any:
		for _, x := range t {
			if markedSensitive(x) {
				return true
			}
		}
	case []any:
		for _, x := range t {
			if markedSensitive(x) {
				return true
			}
		}
	}
	return false
}

func valueString(v any, ok bool) string {
	if !ok || v == nil {
		return ""
	}
	if s, isStr := v.(string); isStr {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
