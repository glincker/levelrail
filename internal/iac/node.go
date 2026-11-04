package iac

import (
	"context"
	"net/http"
	"strings"

	"github.com/GLINCKER/levelrail/internal/placement"
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
	for _, r := range res {
		if r.Kind == KindNode {
			want[r.Name] = true
		}
	}
	if len(want) == 0 {
		return nil
	}
	var all []wireNode
	if err := l.fetch(ctx, "nodes", "list nodes", "/api/v1/nodes", &all); err != nil {
		return err
	}
	for _, n := range all {
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
		p.add(r, ActionNoop, nil, nil, nil)
		return
	}
	var setLabels, setTaints bool
	for _, c := range changes {
		switch topKey(c.Path) {
		case "labels":
			setLabels = true
		case "taints":
			setTaints = true
		}
	}
	p.add(r, ActionUpdate, changes, nil, func(ctx context.Context, x *executor) error {
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
