package iac

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

const cloudUnconfiguredReason = "cloud planner not configured"

// ErrCloudUnavailable marks a planner error caused by missing configuration (a stored
// connection, an engine setting, a binary source); the item fails as unavailable.
var ErrCloudUnavailable = errors.New("cloud provisioning is not configured")

// CloudAttr is one attribute of a proposed cloud change.
type CloudAttr struct {
	Path      string
	Old, New  string
	Sensitive bool
}

// CloudResource is what a planner is asked about, taken from a document.
type CloudResource struct {
	Kind Kind
	Name string
	Spec map[string]any
}

// CloudDiff is a planner answer: action is create, update, delete or noop.
type CloudDiff struct {
	Action string
	// Type is the provider resource type, used by policy checks only.
	Type  string
	Attrs []CloudAttr
	// PlanHash is the sealed engine plan this diff came from.
	PlanHash string
}

// CloudPlanner computes the proposed change for a cloud document. The engine
// implements it; with none set every cloud item fails loudly.
type CloudPlanner interface {
	PlanCloud(ctx context.Context, r CloudResource) (CloudDiff, error)
}

// CloudOutcome is the answer for one document of a stack plan.
type CloudOutcome struct {
	Diff CloudDiff
	Err  error
}

// CloudStackPlanner plans every cloud document of one stack together, so documents that
// refer to each other land in one module. The outcomes line up with the input order.
type CloudStackPlanner interface {
	PlanCloudStack(ctx context.Context, stack string, rs []CloudResource) []CloudOutcome
}

// CloudReadiness is optional on a planner: false means it has no engine yet,
// so even record-only kinds such as CloudAccount must not report unchanged.
type CloudReadiness interface {
	CloudReady() bool
}

func cloudPlannerReady(p CloudPlanner) bool {
	if p == nil {
		return false
	}
	rd, ok := p.(CloudReadiness)
	return !ok || rd.CloudReady()
}

// CloudChange is one entry handed to a PolicyChecker.
type CloudChange struct {
	Address string
	Type    string
	Action  string
}

// CloudRefusal is a policy verdict with a frozen code. An empty Address
// applies to every delete item (the destroy cap).
type CloudRefusal struct {
	Code    string
	Address string
	Message string
}

// PolicyChecker judges a whole cloud change set (cost, IAM, protected delete, destroy cap).
type PolicyChecker interface {
	CheckCloud(changes []CloudChange) []CloudRefusal
}

var (
	cloudMu      sync.RWMutex
	cloudPlanner CloudPlanner
	cloudPolicy  PolicyChecker
)

// SetCloudPlanner installs the planner; nil restores the unconfigured failure.
func SetCloudPlanner(p CloudPlanner) {
	cloudMu.Lock()
	cloudPlanner = p
	cloudMu.Unlock()
}

// SetCloudPolicy installs the policy checker; nil disables the extra checks.
func SetCloudPolicy(c PolicyChecker) {
	cloudMu.Lock()
	cloudPolicy = c
	cloudMu.Unlock()
}

// CloudPlannerInstalled reports whether a planner and a policy checker are both set.
func CloudPlannerInstalled() bool {
	p, c := cloudDeps()
	return p != nil && c != nil
}

func cloudDeps() (CloudPlanner, PolicyChecker) {
	cloudMu.RLock()
	defer cloudMu.RUnlock()
	return cloudPlanner, cloudPolicy
}

// Frozen codes of the cloud apply path.
const (
	CodeApprovalRequired    = "approval_required"
	CodeDestroyNotConfirmed = "destroy_not_confirmed"
	CodeCloudPlanExpired    = "cloud_plan_expired"
	CodeStateDiverged       = "state_diverged"
)

var (
	errCloudApprovalRequired = errors.New(CodeApprovalRequired + ": cloud changes apply only from an approved revision")
	errCloudDestroyConfirm   = errors.New(CodeDestroyNotConfirmed + ": a cloud delete needs confirm_destroy typed with the stack name")
	errCloudNoApplier        = errors.New(CodeUnavailable + ": the cloud engine cannot apply on this server")
)

// IsCloudKind reports whether a kind is provisioned through the cloud engine.
func IsCloudKind(k Kind) bool { return isCloudKind(k) }

// CloudApproval is the approved plan an apply runs. Pins map an item key to the approved
// change, so apply never re-plans and so never replaces the saved engine plan.
type CloudApproval struct {
	Pins           map[string]Change
	ConfirmDestroy bool
}

// CloudApplyRequest asks the engine to run one approved saved plan of a stack.
type CloudApplyRequest struct {
	Stack    string
	Target   CloudResource
	Group    []CloudResource
	PlanHash string
	Destroy  bool
}

// CloudApplyResult reports a state the engine could not store; it never carries state.
type CloudApplyResult struct {
	Diverged     bool
	QuarantineID int64
}

// CloudApplier runs a saved engine plan; a planner with an engine implements it.
type CloudApplier interface {
	ApplyCloud(ctx context.Context, req CloudApplyRequest) (CloudApplyResult, error)
}

// CloudRun is one engine run of an apply, for events: names and ids only.
type CloudRun struct {
	PlanHash     string
	Items        []string
	Diverged     bool
	QuarantineID int64
	Failed       bool
}

// CloudReport collects the engine runs of one apply.
type CloudReport struct {
	mu   sync.Mutex
	runs []CloudRun
}

// Runs returns a copy of the recorded runs.
func (r *CloudReport) Runs() []CloudRun {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]CloudRun(nil), r.runs...)
}

type cloudReportKey struct{}

// WithCloudReport makes an apply record its engine runs into rep.
func WithCloudReport(ctx context.Context, rep *CloudReport) context.Context {
	return context.WithValue(ctx, cloudReportKey{}, rep)
}

type cloudRun struct{ err error }

// secretKeyHints are substrings that mark an attribute path as secret.
var secretKeyHints = []string{"password", "secret", "token", "private", "credential", "apikey", "api_key", "passphrase", "key_material", "user_data"}

func cloudSecretPath(path string) bool {
	l := strings.ToLower(path)
	for _, h := range secretKeyHints {
		if strings.Contains(l, h) {
			return true
		}
	}
	return false
}

func maskAttr(a CloudAttr) FieldChange {
	hide := a.Sensitive || cloudSecretPath(a.Path)
	op := "set"
	switch {
	case a.Old == "" && a.New != "":
		op = "add"
	case a.New == "" && a.Old != "":
		op = "remove"
	}
	f := FieldChange{Path: a.Path, Op: op, Old: a.Old, New: a.New}
	if hide {
		if a.Old != "" {
			f.Old = hiddenValue
		}
		if a.New != "" {
			f.New = hiddenValue
		}
	}
	return f
}

// planCloud maps one cloud document to a plan item. It never skips silently. A stack
// planner defers the document so the whole stack is planned once in flushCloudStack.
func (p *planner) planCloud(r *Resource) {
	planner, _ := cloudDeps()
	if !cloudPlannerReady(planner) {
		p.failCode(r, CodeUnavailable, "%s", cloudUnconfiguredReason)
		return
	}
	if p.opts.Cloud != nil {
		p.pinCloud(r)
		return
	}
	if _, ok := planner.(CloudStackPlanner); ok {
		p.cloudStack = append(p.cloudStack, r)
		return
	}
	diff, err := planner.PlanCloud(p.ctx, CloudResource{Kind: r.Kind, Name: r.Name, Spec: r.Fields})
	p.addCloudItem(r, diff, err)
}

// flushCloudStack plans the deferred cloud documents as one stack named by the apply source.
func (p *planner) flushCloudStack() {
	planner, _ := cloudDeps()
	sp, ok := planner.(CloudStackPlanner)
	if !ok || len(p.cloudStack) == 0 {
		return
	}
	in := make([]CloudResource, len(p.cloudStack))
	for i, r := range p.cloudStack {
		in[i] = CloudResource{Kind: r.Kind, Name: r.Name, Spec: r.Fields}
	}
	out := sp.PlanCloudStack(p.ctx, p.opts.Source, in)
	for i, r := range p.cloudStack {
		if i >= len(out) {
			p.failCode(r, CodeItemFailed, "cloud planner returned no answer")
			continue
		}
		p.addCloudItem(r, out[i].Diff, out[i].Err)
	}
	p.cloudStack = nil
}

func (p *planner) addCloudItem(r *Resource, diff CloudDiff, err error) {
	if errors.Is(err, ErrCloudUnavailable) {
		p.failCode(r, CodeUnavailable, "%v", err)
		return
	}
	if err != nil {
		p.failCode(r, CodeItemFailed, "cloud planner failed: %v", err)
		return
	}
	var action Action
	switch diff.Action {
	case "create":
		action = ActionCreate
	case "update":
		action = ActionUpdate
	case "delete", "destroy":
		action = ActionDelete
	case "noop", "no-op":
		action = ActionNoop
	default:
		p.failCode(r, CodeItemFailed, "cloud planner returned unknown action %q", diff.Action)
		return
	}
	fields := make([]FieldChange, 0, len(diff.Attrs))
	for _, a := range diff.Attrs {
		fields = append(fields, maskAttr(a))
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Path < fields[j].Path })
	it := p.add(r, action, fields, nil, nil)
	it.cloudType = diff.Type
	if action != ActionNoop {
		it.CloudPlanHash = diff.PlanHash
		it.exec = p.cloudExec(it)
	}
}

// pinCloud takes the item from the approved plan; a document absent from it is refused.
func (p *planner) pinCloud(r *Resource) {
	c, ok := p.opts.Cloud.Pins[r.Key()]
	if !ok {
		p.failCode(r, CodeApprovalRequired, "%s is not in the approved plan; plan and request approval again", r.Key())
		return
	}
	fields := c.Fields
	if fields == nil && c.Action != ActionError {
		// A stored plan drops an empty field list; the hash saw an empty, not a nil, list.
		fields = []FieldChange{}
	}
	it := p.add(r, c.Action, fields, nil, nil)
	it.pinned, it.Code, it.Reason = true, c.Code, c.Reason
	if c.Action != ActionNoop && c.Action != ActionError {
		it.CloudPlanHash = c.CloudPlanHash
		it.exec = p.cloudExec(it)
	}
}

// cloudExec runs the approved engine plan of the item once per plan hash. Without an
// approval, a hash or a typed destroy confirmation it refuses before the engine is called.
func (p *planner) cloudExec(it *item) func(ctx context.Context, x *executor) error {
	return func(ctx context.Context, _ *executor) error {
		ap := p.opts.Cloud
		if ap == nil || it.CloudPlanHash == "" {
			return errCloudApprovalRequired
		}
		var names []string
		destroy, anyDelete := true, false
		for _, o := range p.items {
			if o.CloudPlanHash != it.CloudPlanHash || o.exec == nil {
				continue
			}
			names = append(names, string(o.Kind)+"/"+o.Name)
			anyDelete = anyDelete || o.Action == ActionDelete
			destroy = destroy && o.Action == ActionDelete
		}
		if anyDelete && !ap.ConfirmDestroy {
			return errCloudDestroyConfirm
		}
		if p.cloudRuns == nil {
			p.cloudRuns = map[string]*cloudRun{}
		}
		if run, ok := p.cloudRuns[it.CloudPlanHash]; ok {
			return run.err
		}
		run := &cloudRun{}
		p.cloudRuns[it.CloudPlanHash] = run
		planner, _ := cloudDeps()
		ap2, ok := planner.(CloudApplier)
		if !ok || !cloudPlannerReady(planner) {
			run.err = errCloudNoApplier
			return run.err
		}
		var group []CloudResource
		for _, r := range p.res {
			if isCloudKind(r.Kind) {
				group = append(group, CloudResource{Kind: r.Kind, Name: r.Name, Spec: r.Fields})
			}
		}
		res, err := ap2.ApplyCloud(ctx, CloudApplyRequest{Stack: p.opts.Source, Group: group, PlanHash: it.CloudPlanHash, Destroy: destroy,
			Target: CloudResource{Kind: it.Kind, Name: it.Name, Spec: it.res.Fields}})
		if rep, _ := ctx.Value(cloudReportKey{}).(*CloudReport); rep != nil {
			rep.mu.Lock()
			rep.runs = append(rep.runs, CloudRun{PlanHash: it.CloudPlanHash, Items: names, Diverged: res.Diverged, QuarantineID: res.QuarantineID, Failed: err != nil})
			rep.mu.Unlock()
		}
		if res.Diverged {
			err = fmt.Errorf("%s: the cloud state could not be stored and was quarantined as %d: %w", CodeStateDiverged, res.QuarantineID, errors.Join(err))
		}
		run.err = err
		return err
	}
}

// applyCloudPolicy turns policy refusals into failed items with the frozen codes.
func (p *planner) applyCloudPolicy() {
	p.flushCloudStack()
	_, chk := cloudDeps()
	if chk == nil {
		return
	}
	var changes []CloudChange
	byAddr := map[string]*item{}
	for _, it := range p.items {
		if !isCloudKind(it.Kind) || it.Action == ActionError || it.pinned {
			continue
		}
		act := string(it.Action)
		if it.Action == ActionNoop {
			act = "no-op"
		}
		byAddr[it.Key()] = it
		changes = append(changes, CloudChange{Address: it.Key(), Type: it.cloudType, Action: act})
	}
	for _, ref := range chk.CheckCloud(changes) {
		targets := []*item{byAddr[ref.Address]}
		if ref.Address == "" {
			targets = nil
			for _, it := range byAddr {
				if it.Action == ActionDelete {
					targets = append(targets, it)
				}
			}
		}
		for _, it := range targets {
			if it == nil {
				continue
			}
			it.Action, it.Code, it.Reason, it.exec = ActionError, ref.Code, ref.Message, nil
		}
	}
}
