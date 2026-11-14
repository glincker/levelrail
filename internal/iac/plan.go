package iac

import (
	"context"
	"fmt"
	"sort"
)

// Action is what a plan item does.
type Action string

// Plan actions.
const (
	ActionCreate Action = "create"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionNoop   Action = "noop"
	ActionError  Action = "error"
)

// Change is one line of a plan.
type Change struct {
	Kind     Kind          `json:"kind"`
	Name     string        `json:"name"`
	Scope    string        `json:"scope,omitempty"`
	Action   Action        `json:"action"`
	Fields   []FieldChange `json:"fields,omitempty"`
	Kept     []FieldChange `json:"kept,omitempty"`
	Warnings []string      `json:"warnings,omitempty"`
	Reason   string        `json:"reason,omitempty"`
	// Code is the frozen machine code of a failed item; empty otherwise.
	Code string `json:"code,omitempty"`
	// DependsOn lists the resources this item reads from, applied first.
	DependsOn []string `json:"dependsOn,omitempty"`
	Denied    bool     `json:"denied,omitempty"`
	// LiveID is the live identifier a ledger records; it never leaves the process.
	LiveID string `json:"-"`
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
	// CloudPlanHash names the sealed engine plan a cloud item runs; approval binds to it.
	CloudPlanHash string `json:"cloudPlanHash,omitempty"`
	// ComposeDigest covers a compose App's file bytes and inputs; it never leaves the process.
	ComposeDigest string `json:"-"`
}

// Key is the change's stable identity.
func (c Change) Key() string { return resourceKey(c.Kind, c.Scope, c.Name) }

// Summary counts a plan's items by action.
type Summary struct {
	Create int `json:"create"`
	Update int `json:"update"`
	Delete int `json:"delete"`
	Noop   int `json:"noop"`
	Error  int `json:"error"`
}

// Plan is the ordered set of changes needed to reach the desired state.
type Plan struct {
	Source  string   `json:"source,omitempty"`
	Prune   bool     `json:"prune,omitempty"`
	Changes []Change `json:"changes"`
	Summary Summary  `json:"summary"`
	Hash    string   `json:"hash"`
}

// Pending reports whether applying would change anything.
func (p Plan) Pending() bool { return p.Summary.Create+p.Summary.Update+p.Summary.Delete > 0 }

type item struct {
	Change
	res  *Resource
	exec func(ctx context.Context, x *executor) error
	// cloudType is the provider type of a cloud item, for policy checks only.
	cloudType string
	// pinned marks a cloud item taken from an approved plan; policy was judged at plan time.
	pinned bool
}

type planner struct {
	st    *State
	res   []*Resource
	opts  Options
	byKey map[string]*Resource
	items []*item
	// cloudStack holds cloud documents planned together as one stack.
	cloudStack []*Resource
	// cloudRuns runs each approved engine plan once, however many items share it.
	cloudRuns map[string]*cloudRun
}

// PlanFor computes the plan for resources against live state.
func PlanFor(st *State, res []*Resource, opts Options) Plan {
	plan, _ := buildPlan(st, res, opts)
	return plan
}

func buildPlan(st *State, res []*Resource, opts Options) (Plan, []*item) {
	p := &planner{st: st, res: res, opts: opts, byKey: map[string]*Resource{}}
	for _, r := range res {
		p.byKey[r.Key()] = r
	}
	for _, r := range res {
		if p.scopeGate(r) {
			continue
		}
		p.planResource(r)
		p.scopeNote(r)
	}
	if opts.Prune {
		p.planPrune()
	}
	p.applyCloudPolicy()
	sortItems(p.items)
	plan := Plan{Source: opts.Source, Prune: opts.Prune, Changes: make([]Change, len(p.items))}
	for i, it := range p.items {
		if it.Action == ActionError && it.Code == "" {
			it.Code = CodeItemFailed
		}
		plan.Changes[i] = it.Change
		switch it.Action {
		case ActionCreate:
			plan.Summary.Create++
		case ActionUpdate:
			plan.Summary.Update++
		case ActionDelete:
			plan.Summary.Delete++
		case ActionNoop:
			plan.Summary.Noop++
		case ActionError:
			plan.Summary.Error++
		}
	}
	plan.Hash = planHashV2(opts.Source, opts.Prune, plan.Changes, planSecretNames(p.items))
	return plan, p.items
}

func planSecretNames(items []*item) []string {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		if it.res == nil {
			continue
		}
		for _, n := range it.res.SecretRefs {
			k := it.res.Key() + "/" + n
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	return out
}

func sortItems(items []*item) {
	rank := func(it *item) int {
		if it.Action == ActionDelete {
			return 100 + (len(Kinds) - kindRank(it.Kind))
		}
		return kindRank(it.Kind)
	}
	sort.SliceStable(items, func(i, j int) bool {
		ri, rj := rank(items[i]), rank(items[j])
		if ri != rj {
			return ri < rj
		}
		return items[i].Key() < items[j].Key()
	})
}

func (p *planner) add(r *Resource, action Action, fields, kept []FieldChange, exec func(ctx context.Context, x *executor) error) *item {
	it := &item{res: r, exec: exec, Change: Change{Kind: r.Kind, Name: r.Name, Scope: r.Scope, Action: action, Fields: fields, Kept: kept, Warnings: r.Warnings, ComposeDigest: r.ComposeDigest}}
	if r.Doc != nil {
		it.File, it.Line = r.Doc.File, r.Doc.Line
	}
	p.items = append(p.items, it)
	return it
}

func (p *planner) fail(r *Resource, format string, args ...any) {
	p.failCode(r, CodeItemFailed, format, args...)
}

func (p *planner) failCode(r *Resource, code, format string, args ...any) {
	it := p.add(r, ActionError, nil, nil, nil)
	it.Code = code
	it.Reason = fmt.Sprintf(format, args...)
}

func (p *planner) deniedItem(r *Resource, key string) bool {
	if err, ok := p.st.denied[key]; ok {
		it := p.add(r, ActionError, nil, nil, nil)
		if IsUnavailable(err) {
			it.Code = CodeUnavailable
			it.Reason = "not available on this control plane: " + err.Error()
		} else {
			it.Code = CodePermissionDenied
			it.Reason = "not permitted: " + err.Error()
			it.Denied = true
		}
		return true
	}
	return false
}

func (p *planner) planResource(r *Resource) {
	switch r.Kind {
	case KindProject:
		p.planProject(r)
	case KindEnvironment:
		p.planEnvironment(r)
	case KindTag:
		p.planTag(r)
	case KindNode:
		p.planNode(r)
	case KindSetting:
		p.planSetting(r)
	case KindDatabase:
		p.planDatabase(r)
	case KindApp:
		p.planApp(r)
	case KindDomain:
		p.planDomain(r)
	case KindLoadBalancer:
		p.planLB(r)
	case KindPipeline:
		p.planPipeline(r)
	case KindAlertRule:
		p.planAlert(r)
	case KindCloudAccount, KindCloudNetwork, KindCloudInstance, KindCloudBucket:
		p.planCloud(r)
	}
}

// projectKnown reports whether a project exists live or is declared here.
func (p *planner) projectKnown(name string) bool {
	if _, ok := p.st.projects[name]; ok {
		return true
	}
	_, ok := p.byKey[resourceKey(KindProject, "", name)]
	return ok
}

func (p *planner) appKnown(name string) bool {
	if _, ok := p.st.apps[name]; ok {
		return true
	}
	_, ok := p.byKey[resourceKey(KindApp, "", name)]
	return ok
}
