package iac

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

// liveNode is a joined node with the labels and taints a plan compares.
type liveNode struct {
	id     string
	labels map[string]string
	taints []placement.Taint
}

type nodeSpec struct {
	Labels map[string]string `yaml:"labels"`
	Taints []struct {
		Key    string `yaml:"key"`
		Value  string `yaml:"value"`
		Effect string `yaml:"effect"`
	} `yaml:"taints"`
}

// taintKey is "key:effect", unique per node and free of colons in the key.
func taintKey(t placement.Taint) string { return t.Key + ":" + string(t.Effect) }

func taintMap(in []placement.Taint) map[string]any {
	out := make(map[string]any, len(in))
	for _, t := range in {
		out[taintKey(t)] = t.Value
	}
	return out
}

func labelMap(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// buildNode keeps only declared members, so an undeclared one stays unmanaged.
func buildNode(d *Document, s *issueSink) map[string]any {
	var sp nodeSpec
	decodeSpec(d, &sp, s)
	f := map[string]any{}
	if mapValue(d.SpecNode, "labels") != nil {
		if err := placement.ValidateLabels(sp.Labels); err != nil {
			s.at("spec.labels", "%v", err)
		}
		f["labels"] = labelMap(sp.Labels)
	}
	if mapValue(d.SpecNode, "taints") != nil {
		taints := make([]placement.Taint, 0, len(sp.Taints))
		for _, t := range sp.Taints {
			taints = append(taints, placement.Taint{Key: t.Key, Value: t.Value, Effect: placement.TaintEffect(t.Effect)})
		}
		if err := placement.ValidateTaints(taints); err != nil {
			s.at("spec.taints", "%v", err)
		}
		f["taints"] = taintMap(taints)
	}
	return f
}

// nodes reads the node list once when a Node document is present, then the
// labels and taints of each declared node. Duplicate display names are
// recorded so the plan can refuse them.
func (l loader) nodes(ctx context.Context, res []*Resource) error {
	want := map[string]bool{}
	taints := false
	for _, r := range res {
		if r.Kind == KindNode {
			want[r.Name] = true
			_, has := r.Fields["taints"]
			taints = taints || has
		}
	}
	if len(want) == 0 {
		return nil
	}
	if taints {
		if err := l.protectedTaintKeys(ctx); err != nil {
			return err
		}
	}
	var all []wireNode
	if err := l.fetch(ctx, "nodes", "list nodes", "/api/v1/nodes", &all); err != nil {
		return err
	}
	for _, n := range all {
		l.st.nodeNames[n.ID] = n.Name
		if !want[n.Name] {
			continue
		}
		if _, dup := l.st.nodes[n.Name]; dup {
			l.st.nodeDupes[n.Name] = true
			continue
		}
		l.st.nodes[n.Name] = &liveNode{id: n.ID}
	}
	for _, name := range sortedKeys(l.st.nodes) {
		ln := l.st.nodes[name]
		key := resourceKey(KindNode, "", name)
		var lb struct {
			Labels map[string]string `json:"labels"`
		}
		if err := l.fetch(ctx, key, "read labels of node "+name, "/api/v1/nodes/"+esc(ln.id)+"/labels", &lb); err != nil {
			return err
		}
		var tb struct {
			Taints []placement.Taint `json:"taints"`
		}
		if err := l.fetch(ctx, key, "read taints of node "+name, "/api/v1/nodes/"+esc(ln.id)+"/taints", &tb); err != nil {
			return err
		}
		ln.labels, ln.taints = lb.Labels, tb.Taints
	}
	return nil
}

// planNode only configures a node that already joined. It never creates,
// deletes or prunes one; enrollment stays the join token flow.
func (p *planner) planNode(r *Resource) {
	if p.deniedItem(r, "nodes") || p.deniedItem(r, r.Key()) {
		return
	}
	if p.st.nodeDupes[r.Name] {
		p.fail(r, "more than one node is named %q; rename one so the file can name a single node", r.Name)
		return
	}
	if p.nodeRenamed(r) {
		return
	}
	live, ok := p.st.nodes[r.Name]
	if !ok {
		p.fail(r, "node %q does not exist; nodes join through an enrollment token and apply never creates one", r.Name)
		return
	}
	liveMap := map[string]any{}
	if _, want := r.Fields["labels"]; want {
		liveMap["labels"] = labelMap(live.labels)
	}
	if _, want := r.Fields["taints"]; want {
		liveMap["taints"] = taintMap(live.taints)
	}
	changes, _ := diffFields(r.Fields, liveMap, nil, false)
	if len(changes) == 0 {
		p.add(r, ActionNoop, nil, nil, nil).LiveID = live.id
		return
	}
	var setLabels, setTaints bool
	weak := ""
	if _, want := r.Fields["taints"]; want {
		var ok bool
		if weak, ok = p.nodeTaintsGuard(r, live); !ok {
			return
		}
	}
	for _, c := range changes {
		switch topKey(c.Path) {
		case "labels":
			setLabels = true
		case "taints":
			setTaints = true
		}
	}
	it := p.add(r, ActionUpdate, changes, nil, func(ctx context.Context, x *executor) error {
		base := "/api/v1/nodes/" + esc(live.id)
		if setLabels {
			labels := map[string]string{}
			for k, v := range r.Fields["labels"].(map[string]any) {
				labels[k], _ = v.(string)
			}
			if err := x.do(ctx, http.MethodPut, base+"/labels", map[string]any{"labels": labels}, nil); err != nil {
				return err
			}
		}
		if setTaints {
			return x.do(ctx, http.MethodPut, base+"/taints", map[string]any{"taints": taintsFromMap(r.Fields["taints"].(map[string]any))}, nil)
		}
		return nil
	})
	it.LiveID = live.id
	if weak != "" {
		it.Reason = NodeTaintWeakenReason + ": " + weak
	}
}

// NodeTaintWeakenReason starts the reason of a Node change that drops or
// loosens a protected taint under allow_weaken, so reviewers can find it.
const NodeTaintWeakenReason = "weakens a protected taint; allowed by allow_weaken"

// protectedTaintKeys reads nodes.protected_taint_keys through the same GET as
// the Setting planner. A 404 (older control plane) means nothing is protected.
func (l loader) protectedTaintKeys(ctx context.Context) error {
	key := runtimesettings.KeyNodesProtectedTaintKeys
	if _, seen := l.st.settings[key]; seen {
		return nil
	}
	var s liveSetting
	err := l.get(ctx, "/api/v1/settings/runtime/"+esc(key), &s.Setting)
	switch {
	case err == nil:
		s.allowWeaken = allowWeakenFrom(ctx)
		l.st.settings[key] = &s
	case IsNotFound(err):
		l.st.settings[key] = nil
	default:
		return l.st.note(resourceKey(KindSetting, "", key), fmt.Errorf("read setting %s: %w", key, err))
	}
	return nil
}

// nodeTaintsGuard fails r when its taints drop, revalue or loosen a protected
// one without allow_weaken. It returns the weakened keys when allowed. An
// unreadable protected list fails closed, but only for a change that touches a live taint.
func (p *planner) nodeTaintsGuard(r *Resource, live *liveNode) (string, bool) {
	next := taintsFromMap(r.Fields["taints"].(map[string]any))
	key := runtimesettings.KeyNodesProtectedTaintKeys
	if _, denied := p.st.denied[resourceKey(KindSetting, "", key)]; denied {
		if len(weakenedTaints(true, nil, live.taints, next)) > 0 {
			p.fail(r, "node %s: the protected taint keys (%s) cannot be read, and this change drops or loosens a taint; grant settings read or apply from a human root session", r.Name, key)
			return "", false
		}
		return "", true
	}
	ls := p.st.settings[key]
	if ls == nil {
		return "", true
	}
	def, ok := settingDefinition(key)
	if !ok {
		return "", true
	}
	svc, _ := settingMirror(def, ls)
	weak := weakenedTaints(false, svc.List(context.Background(), key), live.taints, next)
	if len(weak) == 0 {
		return "", true
	}
	list := strings.Join(weak, ", ")
	if !ls.allowWeaken {
		p.fail(r, "node %s: the taint change drops, revalues or loosens protected taint key %s; apply with allow_weaken from a human root session, or change it from the dashboard or the CLI", r.Name, list)
		return "", false
	}
	return list, true
}

// taintStrength mirrors the token-side check in internal/api/iam_stack_verbs.go
// (taintsKeepProtected); keep both in step. Unknown effects rank 0.
var taintStrength = map[placement.TaintEffect]int{
	placement.EffectPreferNoSchedule: 1,
	placement.EffectNoSchedule:       2,
	placement.EffectNoExecute:        3,
}

// weakenedTaints lists the live taints with a protected key (case folded) that
// next does not keep with the same value and an effect at least as strong.
// all treats every key as protected. Unknown effects only match exactly.
func weakenedTaints(all bool, protected []string, cur, next []placement.Taint) []string {
	isProtected := map[string]bool{}
	for _, k := range protected {
		isProtected[strings.ToLower(k)] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range cur {
		ck := strings.ToLower(c.Key)
		if !all && !isProtected[ck] {
			continue
		}
		cs := taintStrength[c.Effect]
		kept := false
		for _, n := range next {
			ns := taintStrength[n.Effect]
			if strings.ToLower(n.Key) == ck && n.Value == c.Value && (n.Effect == c.Effect || (cs > 0 && ns >= cs)) {
				kept = true
				break
			}
		}
		if !kept && !seen[c.Key] {
			seen[c.Key] = true
			out = append(out, c.Key)
		}
	}
	sort.Strings(out)
	return out
}

func taintsFromMap(m map[string]any) []placement.Taint {
	out := make([]placement.Taint, 0, len(m))
	for _, k := range sortedKeys(m) {
		key, effect, _ := strings.Cut(k, ":")
		v, _ := m[k].(string)
		out = append(out, placement.Taint{Key: key, Value: v, Effect: placement.TaintEffect(effect)})
	}
	return out
}
