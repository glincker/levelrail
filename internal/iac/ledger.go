package iac

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Ledger is the stack ownership view a plan is checked against (ADR 042).
// With a ledger, ownership and prune come from it alone, never from tags.
type Ledger struct {
	StackID   string
	StackName string
	// ScopeProject is the stack's own project; an admin bound it to the stack,
	// so declaring it claims it without an explicit adopt.
	ScopeProject string
	// Owned maps resource keys this stack owns to their entries.
	Owned map[string]LedgerEntry
	// Foreign maps keys owned by some other stack to that stack's id.
	Foreign map[string]string
	// Adopt lets the stack claim existing resources no stack owns.
	Adopt bool
	// AllowMassPrune confirms a prune above the guard thresholds.
	AllowMassPrune bool
	Guard          PruneGuard
}

// PruneGuard bounds how much of a stack one prune may delete without an
// explicit confirmation.
type PruneGuard struct {
	MaxPercent int
	MaxCount   int
}

// Defaults when the env is unset: more than half of a stack, or more than
// ten items, needs allow_mass_prune.
const (
	defaultPruneMaxPercent = 50
	defaultPruneMaxCount   = 10
)

// PruneGuardFromEnv reads APP_STACK_PRUNE_MAX_PERCENT and
// APP_STACK_PRUNE_MAX_COUNT; zero is valid and means any prune needs the flag.
func PruneGuardFromEnv() PruneGuard {
	g := PruneGuard{MaxPercent: defaultPruneMaxPercent, MaxCount: defaultPruneMaxCount}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("APP_STACK_PRUNE_MAX_PERCENT"))); err == nil && n >= 0 && n <= 100 {
		g.MaxPercent = n
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("APP_STACK_PRUNE_MAX_COUNT"))); err == nil && n >= 0 {
		g.MaxCount = n
	}
	return g
}

// NewLedger builds the view for one stack from its own ledger rows.
func NewLedger(stackID, stackName, scopeProject string, owned []LedgerEntry) *Ledger {
	l := &Ledger{StackID: stackID, StackName: stackName, ScopeProject: scopeProject, Owned: map[string]LedgerEntry{}, Foreign: map[string]string{}, Guard: PruneGuardFromEnv()}
	for _, e := range owned {
		l.Owned[e.Key] = e
	}
	return l
}

// LedgerRef is one (kind, key) whose owner a caller must look up.
type LedgerRef struct {
	Kind Kind
	Key  string
}

// LedgerRefs lists every key the plan checks ownership of: each resource plus
// the parent app of Domain, LoadBalancer, Pipeline and AlertRule documents.
func LedgerRefs(res []*Resource) []LedgerRef {
	seen := map[string]bool{}
	var out []LedgerRef
	addRef := func(k Kind, key string) {
		if !seen[key] {
			seen[key] = true
			out = append(out, LedgerRef{Kind: k, Key: key})
		}
	}
	for _, r := range res {
		addRef(r.Kind, r.Key())
		if app := parentApp(r); app != "" {
			addRef(KindApp, resourceKey(KindApp, "", app))
		}
	}
	return out
}

// ledgerConflictPrefix marks plan items refused by ownership so the stack API
// can answer them with ErrLedgerConflict.
const ledgerConflictPrefix = "ledger conflict: "

// LedgerConflicts returns the keys a plan refused for ownership reasons.
func LedgerConflicts(p Plan) []string {
	var out []string
	for _, c := range p.Changes {
		if c.Action == ActionError && strings.HasPrefix(c.Reason, ledgerConflictPrefix) {
			out = append(out, c.Key())
		}
	}
	return out
}

// massPrunePrefix marks delete items held back by the mass-prune guard.
const massPrunePrefix = "mass prune guard: "

// MassPruneBlocked reports whether the guard held back any delete.
func MassPruneBlocked(p Plan) bool {
	for _, c := range p.Changes {
		if c.Action == ActionError && strings.HasPrefix(c.Reason, massPrunePrefix) {
			return true
		}
	}
	return false
}

type ledgerKey struct{}

// WithLedger makes Load and the planner enforce l. It is a server-side value;
// nothing a client sends can set it.
func WithLedger(ctx context.Context, l *Ledger) context.Context {
	return context.WithValue(ctx, ledgerKey{}, l)
}

func ledgerFrom(ctx context.Context) *Ledger {
	l, _ := ctx.Value(ledgerKey{}).(*Ledger)
	return l
}

func parentApp(r *Resource) string {
	switch r.Kind {
	case KindDomain:
		return stringField(r.Fields, "app")
	case KindLoadBalancer:
		return r.Name
	case KindPipeline, KindAlertRule:
		return r.Scope
	}
	return ""
}

// liveExists reports whether the declared resource already exists live, which
// is what makes an unowned declaration a takeover rather than a create.
func (p *planner) liveExists(r *Resource) bool {
	st := p.st
	switch r.Kind {
	case KindProject:
		_, ok := st.projects[r.Name]
		return ok
	case KindEnvironment:
		_, ok := st.envs[envKey(r.Scope, r.Name)]
		return ok
	case KindTag:
		_, ok := st.tags[r.Name]
		return ok
	case KindDatabase:
		_, ok := st.databases[r.Name]
		return ok
	case KindApp:
		_, ok := st.apps[r.Name]
		return ok
	case KindDomain:
		if a, ok := st.apps[stringField(r.Fields, "app")]; ok {
			for _, d := range a.wire.Domains {
				if d == r.Name {
					return true
				}
			}
		}
		return false
	case KindLoadBalancer:
		return st.lbs[r.Name].Configured
	case KindPipeline:
		_, ok := st.pipelines[r.Scope][r.Name]
		return ok
	case KindAlertRule:
		_, ok := st.alerts[r.Scope][r.Name]
		return ok
	}
	return false
}

func (p *planner) conflict(r *Resource, format string, args ...any) {
	it := p.add(r, ActionError, nil, nil, nil)
	it.Reason = ledgerConflictPrefix + fmt.Sprintf(format, args...)
}

// ledgerGate refuses a declared resource another stack owns, or one that
// exists outside every stack when adopt is off. It reports whether it did.
func (p *planner) ledgerGate(r *Resource) bool {
	l := p.st.ledger
	if l == nil {
		return false
	}
	key := r.Key()
	if owner, ok := l.Foreign[key]; ok {
		p.conflict(r, "%s is owned by stack %s; change it there or release it first", key, owner)
		return true
	}
	if app := parentApp(r); app != "" {
		if owner, ok := l.Foreign[resourceKey(KindApp, "", app)]; ok {
			p.conflict(r, "app %s is owned by stack %s; a %s on it belongs in that stack", app, owner, r.Kind)
			return true
		}
	}
	if _, ok := l.Owned[key]; ok || !p.liveExists(r) {
		return false
	}
	if l.Adopt || p.scopeClaim(r) {
		return false
	}
	p.conflict(r, "%s already exists and no stack owns it; pass adopt to bring it into stack %s", key, l.StackName)
	return true
}

func (p *planner) scopeClaim(r *Resource) bool {
	l := p.st.ledger
	return r.Kind == KindProject && l.ScopeProject != "" && r.Name == l.ScopeProject
}

// ledgerNote flags, on the item just planned for r, that this apply claims an
// existing resource. The item's warnings are copied so r is never mutated.
func (p *planner) ledgerNote(r *Resource) {
	l := p.st.ledger
	if l == nil || len(p.items) == 0 {
		return
	}
	it := p.items[len(p.items)-1]
	if it.res != r || it.Action == ActionError {
		return
	}
	if _, owned := l.Owned[r.Key()]; owned || !p.liveExists(r) {
		return
	}
	msg := "exists outside any stack; this apply adopts it into stack " + l.StackName
	if p.scopeClaim(r) && !l.Adopt {
		msg = "the stack's own project; this apply claims it for stack " + l.StackName
	}
	it.Warnings = append(append([]string(nil), it.Warnings...), msg)
}

// splitLedgerKey turns a ledger key back into kind, scope and name.
func splitLedgerKey(e LedgerEntry) (Kind, string, string, bool) {
	k := e.Kind
	rest, ok := strings.CutPrefix(e.Key, string(k)+"/")
	if !ok || rest == "" {
		return k, "", "", false
	}
	switch k {
	case KindEnvironment, KindPipeline, KindAlertRule:
		scope, name, ok := strings.Cut(rest, "/")
		return k, scope, name, ok && scope != "" && name != ""
	}
	return k, "", rest, !strings.Contains(rest, "/")
}

// pruneTargets lists owned entries absent from the files, sorted by key.
func (l *Ledger) pruneTargets(res []*Resource) []LedgerEntry {
	declared := map[string]bool{}
	for _, r := range res {
		declared[r.Key()] = true
	}
	var out []LedgerEntry
	for _, e := range l.Owned {
		if !declared[e.Key] {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// ledgerApps names the apps Load must read so prune can see owned resources.
func (l *Ledger) ledgerApps(res []*Resource, apps map[string]bool) {
	for _, e := range l.pruneTargets(res) {
		kind, scope, name, ok := splitLedgerKey(e)
		if !ok {
			continue
		}
		switch kind {
		case KindApp, KindLoadBalancer:
			apps[name] = true
		case KindPipeline, KindAlertRule:
			apps[scope] = true
		case KindDomain:
			if e.LiveID != "" {
				apps[e.LiveID] = true
			}
		}
	}
}

// loadLedger reads the live side of every prune target the manifest dropped.
func (l loader) loadLedger(ctx context.Context, lg *Ledger, res []*Resource) error {
	for _, e := range lg.pruneTargets(res) {
		kind, scope, name, ok := splitLedgerKey(e)
		if !ok {
			continue
		}
		var err error
		switch kind {
		case KindEnvironment:
			err = l.envsOf(ctx, scope)
		case KindLoadBalancer:
			err = l.lb(ctx, name)
		case KindPipeline:
			err = l.pipelinesOf(ctx, scope)
		case KindAlertRule:
			err = l.alertsOf(ctx, scope)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// planLedgerPrune deletes owned resources absent from the files. Databases
// and projects are never deleted: they hold data or other stacks' members.
func (p *planner) planLedgerPrune() {
	l := p.st.ledger
	var deletes []*item
	for _, e := range l.pruneTargets(p.res) {
		kind, scope, name, ok := splitLedgerKey(e)
		r := &Resource{Kind: kind, Scope: scope, Name: name}
		if !ok {
			p.fail(r, "ledger key %q is malformed; release it from the stack", e.Key)
			continue
		}
		if kind == KindDatabase || kind == KindProject {
			p.add(r, ActionNoop, nil, nil, nil).Reason = fmt.Sprintf("absent from the files, but a %s is never pruned; it stays owned by stack %s until the stack forgets it", strings.ToLower(string(kind)), l.StackName)
			continue
		}
		if p.deniedItem(r, pruneReadKey(kind, scope, name, e.LiveID)) {
			continue
		}
		exec, live := p.pruneExec(kind, scope, name, e.LiveID)
		it := p.add(r, ActionDelete, nil, nil, exec)
		if !live {
			it.exec = nil
			it.Reason = "owned by stack " + l.StackName + " but already gone; released from the ledger"
			continue
		}
		it.Reason = "owned by stack " + l.StackName + " and absent from the files"
		deletes = append(deletes, it)
	}
	p.guardPrune(deletes)
}

// guardPrune turns every delete into an error when the prune is too large and
// was not confirmed, so ContinueOnError cannot run part of it.
func (p *planner) guardPrune(deletes []*item) {
	l := p.st.ledger
	n, total := len(deletes), len(l.Owned)
	if n == 0 || l.AllowMassPrune {
		return
	}
	if n <= l.Guard.MaxCount && n*100 <= l.Guard.MaxPercent*total {
		return
	}
	reason := fmt.Sprintf("%sthis prune would delete %d of the %d resources stack %s owns (limits: %d items, %d%%); pass allow_mass_prune to confirm",
		massPrunePrefix, n, total, l.StackName, l.Guard.MaxCount, l.Guard.MaxPercent)
	for _, it := range deletes {
		it.Action, it.exec, it.Reason = ActionError, nil, reason
	}
}

func pruneReadKey(kind Kind, scope, name, liveID string) string {
	switch kind {
	case KindEnvironment:
		return "environments/" + scope
	case KindTag:
		return "tags"
	case KindPipeline:
		return "pipelines/" + scope
	case KindAlertRule:
		return "alerts/" + scope
	case KindLoadBalancer:
		return resourceKey(KindLoadBalancer, "", name)
	case KindDomain:
		return resourceKey(KindApp, "", liveID)
	}
	return resourceKey(KindApp, "", name)
}

// pruneExec returns the delete for one owned resource and whether it is live.
func (p *planner) pruneExec(kind Kind, scope, name, liveID string) (func(ctx context.Context, x *executor) error, bool) {
	st := p.st
	del := func(path string) func(ctx context.Context, x *executor) error {
		return func(ctx context.Context, x *executor) error { return x.do(ctx, http.MethodDelete, path, nil, nil) }
	}
	switch kind {
	case KindApp:
		_, ok := st.apps[name]
		return del("/api/v1/apps/" + esc(name)), ok
	case KindLoadBalancer:
		return del("/api/v1/apps/" + esc(name) + "/loadbalancer"), st.lbs[name].Configured
	case KindPipeline:
		_, ok := st.pipelines[scope][name]
		return del("/api/v1/apps/" + esc(scope) + "/pipelines/" + esc(name)), ok
	case KindAlertRule:
		a, ok := st.alerts[scope][name]
		return del("/api/v1/apps/" + esc(scope) + "/alerts/" + esc(a.ID)), ok
	case KindEnvironment:
		e, ok := st.envs[envKey(scope, name)]
		return del("/api/v1/environments/" + esc(e.ID)), ok
	case KindTag:
		t, ok := st.tags[name]
		return del("/api/v1/tags/" + esc(t.ID)), ok
	case KindDomain:
		live := false
		if a, ok := st.apps[liveID]; ok {
			for _, d := range a.wire.Domains {
				live = live || d == name
			}
		}
		return func(ctx context.Context, x *executor) error { return x.removeDomain(ctx, liveID, name) }, live
	}
	return nil, false
}

func (x *executor) removeDomain(ctx context.Context, app, host string) error {
	var raw map[string]any
	if err := x.do(ctx, http.MethodGet, "/api/v1/apps/"+esc(app), nil, &raw); err != nil {
		return err
	}
	domains, _ := raw["domains"].([]any)
	kept := make([]any, 0, len(domains))
	for _, d := range domains {
		if d != host {
			kept = append(kept, d)
		}
	}
	if len(kept) == len(domains) {
		return nil
	}
	raw["domains"] = kept
	return x.do(ctx, http.MethodPut, "/api/v1/apps/"+esc(app), raw, nil)
}

// LedgerUpdate is what one apply changes in the ledger.
type LedgerUpdate struct {
	Claim   []LedgerEntry
	Release []LedgerEntry
}

// LedgerChanges derives claims and releases from an apply. Only declared
// resources are claimed, and nothing is claimed from a plan refused for errors.
func LedgerChanges(stackID string, rev int, res []*Resource, out ApplyResult) LedgerUpdate {
	byKey := make(map[string]*Resource, len(res))
	for _, r := range res {
		byKey[r.Key()] = r
	}
	var u LedgerUpdate
	for _, it := range out.Results {
		e := LedgerEntry{StackID: stackID, Kind: it.Kind, Key: it.Key(), LiveID: it.Name, SinceRevision: rev}
		switch {
		case it.Action == ActionDelete && (it.Status == StatusApplied || it.Status == StatusNoop):
			u.Release = append(u.Release, e)
		case it.Action == ActionDelete || it.Action == ActionError:
		case it.Status == StatusApplied || (it.Status == StatusNoop && out.Plan.Summary.Error == 0):
			r, declared := byKey[e.Key]
			if !declared {
				continue
			}
			if r.Kind == KindDomain {
				e.LiveID = stringField(r.Fields, "app")
			}
			u.Claim = append(u.Claim, e)
		}
	}
	return u
}
