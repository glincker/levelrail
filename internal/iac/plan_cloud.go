package iac

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
)

const cloudUnconfiguredReason = "cloud planner not configured"

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
}

// CloudPlanner computes the proposed change for a cloud document. The engine
// implements it; with none set every cloud item fails loudly.
type CloudPlanner interface {
	PlanCloud(r CloudResource) (CloudDiff, error)
}

// CloudOutcome is the answer for one document of a stack plan.
type CloudOutcome struct {
	Diff CloudDiff
	Err  error
}

// CloudStackPlanner plans every cloud document of one stack together, so documents that
// refer to each other land in one module. The outcomes line up with the input order.
type CloudStackPlanner interface {
	PlanCloudStack(stack string, rs []CloudResource) []CloudOutcome
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

var errCloudApply = errors.New("cloud apply is not available yet")

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
	if planner == nil {
		p.failCode(r, CodeUnavailable, "%s", cloudUnconfiguredReason)
		return
	}
	if _, ok := planner.(CloudStackPlanner); ok {
		p.cloudStack = append(p.cloudStack, r)
		return
	}
	diff, err := planner.PlanCloud(CloudResource{Kind: r.Kind, Name: r.Name, Spec: r.Fields})
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
	out := sp.PlanCloudStack(p.opts.Source, in)
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
	var exec func(ctx context.Context, x *executor) error
	if action != ActionNoop {
		exec = func(context.Context, *executor) error { return errCloudApply }
	}
	it := p.add(r, action, fields, nil, exec)
	it.cloudType = diff.Type
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
		if !isCloudKind(it.Kind) || it.Action == ActionError {
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
