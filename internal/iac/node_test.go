package iac

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/placement"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
)

const nodeDoc = `version: 1
kind: Node
metadata:
  name: worker-1
spec:
  labels:
    zone: a
  taints:
    - {key: gpu, value: "true", effect: NoSchedule}
`

// addNodes registers node routes on the fake. Node ids are n1, n2 and so on;
// PUT replaces the stored labels and taints.
func addNodes(f *fakeCP, names ...string) (labels, taints map[string]any) {
	labels, taints = map[string]any{}, map[string]any{}
	nodes := []any{}
	for i, n := range names {
		nodes = append(nodes, map[string]any{"id": "n" + string(rune('1'+i)), "name": n})
	}
	f.route("GET", `/api/v1/nodes`, func(_ []string, _ map[string]any) (any, error) { return nodes, nil })
	f.route("GET", `/api/v1/nodes/([^/]+)/labels`, func(m []string, _ map[string]any) (any, error) {
		return map[string]any{"labels": labels[m[1]]}, nil
	})
	f.route("PUT", `/api/v1/nodes/([^/]+)/labels`, func(m []string, b map[string]any) (any, error) {
		labels[m[1]] = b["labels"]
		return b, nil
	})
	f.route("GET", `/api/v1/nodes/([^/]+)/taints`, func(m []string, _ map[string]any) (any, error) {
		return map[string]any{"taints": taints[m[1]]}, nil
	})
	f.route("PUT", `/api/v1/nodes/([^/]+)/taints`, func(m []string, b map[string]any) (any, error) {
		taints[m[1]] = b["taints"]
		return b, nil
	})
	return labels, taints
}

func TestNodeApplyConfiguresAndReplansClean(t *testing.T) {
	f := newFake()
	labels, taints := addNodes(f, "worker-1")
	opts := Options{Source: "git"}
	out := applyAll(t, f, nodeDoc, opts)
	if out.Applied != 1 || out.Failed != 0 {
		t.Fatalf("apply: %+v", out)
	}
	if labels["n1"] == nil || taints["n1"] == nil {
		t.Fatalf("labels %v taints %v", labels, taints)
	}
	if p := planFor(t, f, nodeDoc, opts); quiet(p) != 0 {
		t.Fatalf("replan not empty: %+v", p.Changes)
	}
	cleared := "version: 1\nkind: Node\nmetadata:\n  name: worker-1\nspec:\n  taints: []\n"
	p := planFor(t, f, cleared, opts)
	if p.Summary.Update != 1 || len(p.Changes[0].Fields) != 1 || p.Changes[0].Fields[0].Op != OpRemove {
		t.Fatalf("clearing taints only should remove one taint and leave labels: %+v", p.Changes)
	}
}

func TestNodeMissingOrDuplicateIsAnErrorNeverCreate(t *testing.T) {
	f := newFake()
	addNodes(f, "other")
	p := planFor(t, f, nodeDoc, Options{Source: "git"})
	if p.Summary.Error != 1 || !strings.Contains(p.Changes[0].Reason, "does not exist") {
		t.Fatalf("missing node: %+v", p.Changes)
	}
	out := applyAll(t, f, nodeDoc, Options{Source: "git"})
	if out.Applied != 0 {
		t.Fatalf("apply must change nothing: %+v", out)
	}
	for _, w := range f.writes {
		if strings.Contains(w, "/nodes") {
			t.Fatalf("unexpected node write %s", w)
		}
	}

	d := newFake()
	addNodes(d, "worker-1", "worker-1")
	p = planFor(t, d, nodeDoc, Options{Source: "git"})
	if p.Summary.Error != 1 || !strings.Contains(p.Changes[0].Reason, "more than one node") {
		t.Fatalf("duplicate node: %+v", p.Changes)
	}
}

func TestNodeNeverPrunedAndDeniedIsReported(t *testing.T) {
	f := newFake()
	addNodes(f, "worker-1")
	opts := Options{Source: "git", Prune: true}
	applyAll(t, f, nodeDoc, opts)
	empty := "version: 1\nkind: Tag\nmetadata:\n  name: t\n"
	p := planFor(t, f, empty, opts)
	for _, c := range p.Changes {
		if c.Kind == KindNode {
			t.Fatalf("a removed Node document must not plan anything: %+v", c)
		}
	}

	g := newFake()
	addNodes(g, "worker-1")
	g.deny = func(method, path string) bool { return method == http.MethodGet && path == "/api/v1/nodes" }
	p = planFor(t, g, nodeDoc, Options{Source: "git"})
	if p.Summary.Error != 1 || !p.Changes[0].Denied {
		t.Fatalf("denied list: %+v", p.Changes)
	}
}

func TestNodeInvalidTaintIsABuildIssue(t *testing.T) {
	bad := strings.Replace(nodeDoc, "NoSchedule", "Evict", 1)
	docs, issues := ParseDocuments([]Source{{Name: "n.yaml", Data: []byte(bad)}})
	if len(issues) == 0 {
		_, issues = Build(docs, Options{})
	}
	if len(issues) == 0 {
		t.Fatal("an invalid effect must be rejected")
	}
}

func TestExportDatabaseFilter(t *testing.T) {
	f := newFake()
	doc := "version: 1\nkind: Database\nmetadata: {name: orders}\nspec: {engine: postgres}\n---\nversion: 1\nkind: Database\nmetadata: {name: other}\nspec: {engine: postgres}\n"
	applyAll(t, f, doc, Options{})
	out, err := Export(context.Background(), f, ExportOptions{Database: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Files) != 1 || !strings.Contains(out.Join(), "name: orders") {
		t.Fatalf("want only orders:\n%s", out.Join())
	}
	_, err = Export(context.Background(), f, ExportOptions{Database: "nope"})
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusNotFound {
		t.Fatalf("want 404, got %v", err)
	}
}

func TestExportNodeFilter(t *testing.T) {
	f := newFake()
	addNodes(f, "worker-1", "worker-2")
	opts := Options{Source: "git"}
	applyAll(t, f, nodeDoc, opts)
	out, err := Export(context.Background(), f, ExportOptions{Node: "worker-1"})
	if err != nil {
		t.Fatal(err)
	}
	j := out.Join()
	if len(out.Files) != 1 || !strings.Contains(j, "kind: Node") || !strings.Contains(j, "zone: a") || !strings.Contains(j, "key: gpu") {
		t.Fatalf("node export:\n%s", j)
	}
	if p := planFor(t, f, j, opts); quiet(p) != 0 {
		t.Fatalf("export does not replan clean: %+v", p.Changes)
	}
	_, err = Export(context.Background(), f, ExportOptions{Node: "ghost"})
	var se *StatusError
	if !errors.As(err, &se) || se.Status != http.StatusNotFound {
		t.Fatalf("want 404, got %v", err)
	}
}

// protectTaints serves the settings routes with nodes.protected_taint_keys set.
func protectTaints(f *fakeCP, keys ...string) {
	live := addSettings(f)
	raw, _ := json.Marshal(keys)
	s := live[runtimesettings.KeyNodesProtectedTaintKeys]
	s.Value, s.Source = raw, runtimesettings.SourceSaved
}

func nodeTaintsDoc(taints string) string {
	return "version: 1\nkind: Node\nmetadata:\n  name: worker-1\nspec:\n  taints:\n" + taints
}

// protectedNode applies nodeDoc (taint gpu=true:NoSchedule) and then protects keys.
func protectedNode(t *testing.T, keys ...string) *fakeCP {
	f := newFake()
	addNodes(f, "worker-1")
	applyAll(t, f, nodeDoc, Options{Source: "git"})
	protectTaints(f, keys...)
	return f
}

func TestNodeProtectedTaintWeakeningIsAPlanError(t *testing.T) {
	cases := map[string]string{
		"removed":  "    []\n",
		"revalued": "    - {key: gpu, value: \"false\", effect: NoSchedule}\n",
		"loosened": "    - {key: gpu, value: \"true\", effect: PreferNoSchedule}\n",
		"recased":  "    - {key: gpu, value: \"true\", effect: PreferNoSchedule}\n    - {key: ssd, value: \"y\", effect: NoSchedule}\n",
	}
	for name, taints := range cases {
		t.Run(name, func(t *testing.T) {
			f := protectedNode(t, "GPU")
			p := planFor(t, f, nodeTaintsDoc(taints), Options{Source: "git"})
			if p.Summary.Error != 1 || !strings.Contains(p.Changes[0].Reason, "allow_weaken") || !strings.Contains(p.Changes[0].Reason, "gpu") {
				t.Fatalf("%s: %+v", name, p.Changes)
			}
			if out := applyAll(t, f, nodeTaintsDoc(taints), Options{Source: "git"}); out.Applied != 0 {
				t.Fatalf("%s: a weakening apply must change nothing: %+v", name, out)
			}
		})
	}
}

func TestNodeProtectedTaintKeptOrStrongerOrUnprotectedPlans(t *testing.T) {
	f := protectedNode(t, "gpu")
	stronger := nodeTaintsDoc("    - {key: gpu, value: \"true\", effect: NoExecute}\n    - {key: ssd, value: \"y\", effect: NoSchedule}\n")
	if p := planFor(t, f, stronger, Options{Source: "git"}); p.Summary.Error != 0 || p.Summary.Update != 1 {
		t.Fatalf("a stronger effect must plan: %+v", p.Changes)
	}
	g := protectedNode(t, "other")
	if p := planFor(t, g, nodeTaintsDoc("    []\n"), Options{Source: "git"}); p.Summary.Error != 0 || p.Summary.Update != 1 {
		t.Fatalf("an unprotected key may be removed: %+v", p.Changes)
	}
}

func TestNodeProtectedTaintAllowWeaken(t *testing.T) {
	f := protectedNode(t, "gpu")
	opts := Options{Source: "git"}
	res := mustBuild(t, nodeTaintsDoc("    []\n"), opts)
	st, err := Load(WithAllowWeaken(context.Background()), f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	p := PlanFor(st, res, opts)
	if p.Summary.Error != 0 || p.Summary.Update != 1 || !strings.HasPrefix(p.Changes[0].Reason, NodeTaintWeakenReason) {
		t.Fatalf("allow_weaken must let the removal plan: %+v", p.Changes)
	}
}

func TestNodeProtectedTaintUnreadableFailsClosed(t *testing.T) {
	f := protectedNode(t, "gpu")
	f.deny = func(_, path string) bool { return strings.HasPrefix(path, "/api/v1/settings/") }
	if p := planFor(t, f, nodeTaintsDoc("    []\n"), Options{Source: "git"}); p.Summary.Error != 1 {
		t.Fatalf("an unreadable list must fail a removal: %+v", p.Changes)
	}
	add := nodeTaintsDoc("    - {key: gpu, value: \"true\", effect: NoSchedule}\n    - {key: ssd, value: \"y\", effect: NoSchedule}\n")
	if p := planFor(t, f, add, Options{Source: "git"}); p.Summary.Error != 0 || p.Summary.Update != 1 {
		t.Fatalf("an addition needs no list: %+v", p.Changes)
	}
}

func TestNodeProtectedTaintUnknownEffectFailsClosed(t *testing.T) {
	cur := []placement.Taint{{Key: "GPU", Value: "true", Effect: "Custom"}}
	if w := weakenedTaints(false, []string{"gpu"}, cur, []placement.Taint{{Key: "gpu", Value: "true", Effect: placement.EffectNoExecute}}); len(w) != 1 {
		t.Fatalf("an unknown live effect is kept only by the same effect: %v", w)
	}
	if w := weakenedTaints(false, []string{"gpu"}, cur, []placement.Taint{{Key: "gpu", Value: "true", Effect: "Custom"}}); len(w) != 0 {
		t.Fatalf("the same effect keeps the taint: %v", w)
	}
	strong := []placement.Taint{{Key: "gpu", Value: "true", Effect: placement.EffectNoSchedule}}
	if w := weakenedTaints(false, []string{"GPU"}, strong, []placement.Taint{{Key: "gpu", Value: "true", Effect: "Bogus"}}); len(w) != 1 {
		t.Fatalf("an unknown next effect never keeps a taint: %v", w)
	}
}
