package cloudiac

import (
	"context"
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
)

// Sentinels callers match with errors.Is.
var (
	ErrStateLocked    = errors.New("cloudiac: cloud state is locked by another run")
	ErrNotImplemented = errors.New("cloudiac: step is not implemented yet")
)

const (
	tofuStateFile  = "terraform.tfstate"
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
	Policy  func() Policy
	Cost    func() CostPolicy
	Prices  PriceTable
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

// TofuEngine is the OpenTofu Engine. Only the plan path exists; apply and destroy come later.
type TofuEngine struct {
	cfg  TofuConfig
	deps TofuDeps
	now  func() time.Time
}

// NewTofuEngine builds the engine; missing pieces surface at Plan time, not here.
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

// PlanResult is the typed plan with its cost verdict.
type PlanResult struct {
	Changes  []PlanChange
	Cost     CostEstimate
	Warnings []string
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
	res := Result{Step: StepPlan, Log: strings.Join(append([]string{pr.Summary()}, pr.Warnings...), "\n")}
	for _, c := range pr.Changes {
		res.Changes = append(res.Changes, Change{Address: c.Address, Action: c.Action})
	}
	return res, nil
}

// Apply is not implemented in this phase.
func (e *TofuEngine) Apply(context.Context, ModuleSpec, CredentialHandle) (Result, error) {
	return Result{}, &Error{Kind: ErrUnsupported, Step: StepApply, Message: "apply is not available yet", Err: ErrNotImplemented}
}

// Destroy is not implemented in this phase.
func (e *TofuEngine) Destroy(context.Context, ModuleSpec, CredentialHandle) (Result, error) {
	return Result{}, &Error{Kind: ErrUnsupported, Step: StepDestroy, Message: "destroy is not available yet", Err: ErrNotImplemented}
}

func planErr(kind ErrorKind, msg string, err error) error {
	return &Error{Kind: kind, Step: StepPlan, Message: msg, Err: err}
}

// PlanDetailed runs init, plan and show under the state lease and returns the typed plan.
// The account is the credential handle id and the stack is the module name.
func (e *TofuEngine) PlanDetailed(ctx context.Context, spec ModuleSpec, cred CredentialHandle) (_ PlanResult, err error) {
	d := e.deps
	cfg := e.cfg
	if cfg.Live != nil {
		cfg = cfg.Live(cfg)
	}
	if err := cfg.missing(d); err != nil {
		return PlanResult{}, err
	}
	account, stack := cred.ID, spec.Name
	if account == "" || stack == "" || len(spec.Files) == 0 {
		return PlanResult{}, planErr(ErrInvalidSpec, "account, stack and module files are required", nil)
	}
	timeout := cfg.Timeout()
	if timeout <= 0 {
		return PlanResult{}, planErr(ErrInternal, "engine timeout is not set", ErrNotConfigured)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin, err := d.Binary.Ensure(ctx)
	if err != nil {
		return PlanResult{}, planErr(ErrInternal, "OpenTofu binary is not available: "+err.Error(), err)
	}
	holder := cfg.Holder
	if holder == "" {
		holder = "levelrail-plan"
	}
	lock, err := d.Locks.AcquireCloudStateLock(ctx, account, stack, holder, timeout+time.Minute, e.now())
	if errors.Is(err, store.ErrCloudStateLocked) {
		return PlanResult{}, planErr(ErrLockHeld, "another run holds the state lock for this stack", ErrStateLocked)
	}
	if err != nil {
		return PlanResult{}, planErr(ErrInternal, "state lock: "+err.Error(), err)
	}
	defer func() { _ = d.Locks.ReleaseCloudStateLock(context.WithoutCancel(ctx), lock) }()

	state, _, err := d.State.Load(ctx, account, stack)
	if err != nil {
		return PlanResult{}, planErr(ErrInternal, "load state: "+err.Error(), err)
	}
	oc, err := d.Creds.ResolveOracle(ctx, cred)
	if err != nil {
		return PlanResult{}, planErr(ErrCredential, "credential could not be resolved", err)
	}
	run, err := NewRunDir(cfg.WorkBase, cfg.User)
	if err != nil {
		return PlanResult{}, planErr(ErrInternal, "run dir: "+err.Error(), err)
	}
	defer run.Cleanup()
	var warnings []string
	if !run.OnTmpfs {
		warnings = append(warnings, "warning: the cloud run directory is not on tmpfs, so the plaintext state touched disk during the run")
	}
	if free, ferr := FreeBytes(run.Root); ferr == nil && free < int64(cfg.Limits.WorkTmpMB)<<20 {
		return PlanResult{}, planErr(ErrInternal, "not enough free space for the cloud run directory", ErrLowDisk)
	}
	moduleDir := run.ModuleDir()
	if err := writeModule(moduleDir, spec.Files, state); err != nil {
		return PlanResult{}, planErr(ErrInvalidSpec, err.Error(), err)
	}
	credMount := SandboxCredentialDir + "/credential"
	files, err := cloudconn.MaterializeOracle(run.Root, credMount, oc)
	if err != nil {
		return PlanResult{}, planErr(ErrCredential, "credential could not be prepared", err)
	}
	defer func() { _ = files.Close() }()
	sensitive := SensitiveValues(state)
	redact := func(s string) string {
		return RedactStepOutput(s, stderrTailSize, files.Redact, sensitive, state)
	}

	sb, err := BuildSandboxSpec(SandboxInput{
		Runtime: cfg.Runtime, Image: cfg.Image, User: cfg.User, Step: StepPlan,
		ModuleDir: moduleDir, RunDir: run.Shared(), TofuBinary: bin, PluginDir: cfg.PluginDir,
		Credential: CredentialFile{HostPath: files.Dir(), EnvName: credEnvName},
		Network:    cfg.Network, Limits: cfg.Limits,
		Env: map[string]string{"OCI_CONFIG_FILE": credMount + "/" + cloudconn.OracleConfigFileName},
	})
	if err != nil {
		return PlanResult{}, planErr(ErrInvalidSpec, err.Error(), err)
	}
	chdir := "-chdir=" + SandboxModuleDir
	steps := [][]string{
		{chdir, "init", "-input=false", "-no-color"},
		sb.Args,
		{chdir, "show", "-json", "-no-color", SandboxStateDir + "/plan.tfplan"},
	}
	var show RunOutput
	for i, args := range steps {
		step := sb
		step.Args = args
		out, rerr := d.Runner.Run(ctx, step)
		if rerr != nil {
			if i == len(steps)-1 {
				sensitive = append(sensitive, SensitiveValues(out.Stdout)...)
			}
			return PlanResult{}, stepFailure(ctx, args[1], out, rerr, redact)
		}
		if i == len(steps)-1 {
			show = out
		}
	}
	if show.Truncated {
		return PlanResult{}, planErr(ErrInternal, "plan output exceeds the output cap", nil)
	}
	changes, err := parsePlanJSON(show.Stdout)
	if err != nil {
		return PlanResult{}, planErr(ErrProvider, "plan output could not be read", err)
	}
	pr, err := e.judge(ctx, cfg, account, changes)
	pr.Warnings = warnings
	return pr, err
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

func stepFailure(ctx context.Context, name string, out RunOutput, err error, redact func(string) string) error {
	if ctx.Err() != nil {
		return planErr(ErrCanceled, name+" stopped: "+ctx.Err().Error(), ctx.Err())
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
	return planErr(ErrProvider, msg, errors.New(redact(err.Error())))
}

// judge applies the change set policy and the cost limit before a plan is returned.
func (e *TofuEngine) judge(ctx context.Context, cfg TofuConfig, account string, changes []PlanChange) (PlanResult, error) {
	rc := make([]ResourceChange, 0, len(changes))
	for _, c := range changes {
		rc = append(rc, ResourceChange{Address: c.Address, Type: c.Type, Action: c.Action})
	}
	var existing int64
	if cfg.ExistingSpend != nil {
		existing = cfg.ExistingSpend(ctx, account)
	}
	est := cfg.Cost().Estimate(cfg.Prices, rc, existing)
	refusals := append(cfg.Policy().Check(rc), est.Refusals...)
	res := PlanResult{Changes: changes, Cost: est}
	if len(refusals) > 0 {
		codes := make([]string, 0, len(refusals))
		for _, r := range refusals {
			codes = append(codes, r.Error())
		}
		return res, planErr(ErrPolicyRefused, strings.Join(codes, "; "), RefusalsError(refusals))
	}
	return res, nil
}

// writeModule writes rendered files and the plaintext state into a private dir that the caller removes.
func writeModule(dir string, files map[string][]byte, state []byte) error {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return err
	}
	for name, body := range files {
		if name == "" || name != filepath.Base(name) || strings.HasPrefix(name, ".") || name == tofuStateFile {
			return fmt.Errorf("module file name %q is not allowed", name)
		}
		if err := os.WriteFile(filepath.Join(dir, name), body, 0o600); err != nil {
			return err
		}
	}
	if len(state) > 0 {
		return os.WriteFile(filepath.Join(dir, tofuStateFile), state, 0o600)
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
