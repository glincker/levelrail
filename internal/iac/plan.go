package iac

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	Denied   bool          `json:"denied,omitempty"`
	File     string        `json:"file,omitempty"`
	Line     int           `json:"line,omitempty"`
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
}

type planner struct {
	st    *State
	res   []*Resource
	opts  Options
	byKey map[string]*Resource
	items []*item
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
		p.planResource(r)
	}
	if opts.Prune {
		p.planPrune()
	}
	sortItems(p.items)
	plan := Plan{Source: opts.Source, Prune: opts.Prune, Changes: make([]Change, len(p.items))}
	for i, it := range p.items {
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
	plan.Hash = planHash(plan.Changes)
	return plan, p.items
}

func planHash(changes []Change) string {
	raw, _ := json.Marshal(changes)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:8])
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
	it := &item{res: r, exec: exec, Change: Change{Kind: r.Kind, Name: r.Name, Scope: r.Scope, Action: action, Fields: fields, Kept: kept, Warnings: r.Warnings}}
	if r.Doc != nil {
		it.File, it.Line = r.Doc.File, r.Doc.Line
	}
	p.items = append(p.items, it)
	return it
}

func (p *planner) fail(r *Resource, format string, args ...any) {
	it := p.add(r, ActionError, nil, nil, nil)
	it.Reason = fmt.Sprintf(format, args...)
}

func (p *planner) deniedItem(r *Resource, key string) bool {
	if err, ok := p.st.denied[key]; ok {
		it := p.add(r, ActionError, nil, nil, nil)
		if IsUnavailable(err) {
			it.Reason = "not available on this control plane: " + err.Error()
		} else {
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
