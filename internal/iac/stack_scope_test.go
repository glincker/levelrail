package iac

import (
	"context"
	"strings"
	"testing"
)

func TestStackScopeValidate(t *testing.T) {
	cases := []struct {
		name    string
		scope   StackLevel
		project string
		env     string
		wantErr string
	}{
		{"project with project", LevelProject, "shop", "", ""},
		{"empty scope is project", "", "shop", "", ""},
		{"project without project", LevelProject, "", "", "project is required"},
		{"install without project", LevelInstall, "", "", ""},
		{"install with project", LevelInstall, "shop", "", "no project or environment"},
		{"install with environment", LevelInstall, "", "prod", "no project or environment"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			sc, err := ParseStackLevel(string(c.scope))
			if err != nil {
				t.Fatal(err)
			}
			err = ValidateStackLevel(sc, c.project, c.env)
			if (err == nil) != (c.wantErr == "") || (err != nil && !strings.Contains(err.Error(), c.wantErr)) {
				t.Fatalf("got %v, want %q", err, c.wantErr)
			}
		})
	}
	if _, err := ParseStackLevel("cluster"); err == nil {
		t.Fatal("unknown scope accepted")
	}
	spec := StackSpec{Scope: LevelInstall}
	if err := spec.Validate(); err != nil || spec.Drift != DefaultDriftPolicy {
		t.Fatalf("install spec: %v %+v", err, spec)
	}
	st := Stack{Name: "nodes-eu", Scope: LevelInstall}
	if err := st.Validate(); err != nil {
		t.Fatalf("install stack: %v", err)
	}
	st.ProjectID = "shop"
	if err := st.Validate(); err == nil {
		t.Fatal("install stack with a project accepted")
	}
}

func installPlan(t *testing.T, f *fakeCP, l *Ledger, src string, opts Options) Plan {
	t.Helper()
	res := mustBuild(t, src, opts)
	ctx := WithLedger(context.Background(), l)
	st, err := Load(ctx, f, res, opts)
	if err != nil {
		t.Fatal(err)
	}
	return PlanFor(st, res, opts)
}

func TestInstallStackLedgerKinds(t *testing.T) {
	opts := Options{Source: "nodes-eu"}
	f := newFake()
	addNodes(f, "worker-1")

	// Only Node and Setting documents are allowed in an install stack.
	l := NewInstallLedger("stk_1", "nodes-eu", nil)
	p := installPlan(t, f, l, nodeDoc+"---\n"+appDoc("web", "nginx"), opts)
	errs := errorsByKey(p)
	if !strings.Contains(errs["App/web"], "may own only Node and Setting") || len(errs) != 1 {
		t.Fatalf("errors: %v", errs)
	}

	// A project stack holding a Node still plans, with a warning.
	pl := NewLedger("stk_2", "shop", "shop", nil)
	p = installPlan(t, f, pl, nodeDoc, opts)
	if p.Summary.Error != 0 || len(p.Changes[0].Warnings) != 1 || !strings.Contains(p.Changes[0].Warnings[0], "install stack") {
		t.Fatalf("project stack node: %+v", p.Changes)
	}
	if p = installPlan(t, f, l, nodeDoc, opts); len(p.Changes[0].Warnings) != 0 {
		t.Fatalf("install stack must not warn: %+v", p.Changes)
	}

	// The ledger records the node id, and prune releases without touching the node.
	res := mustBuild(t, nodeDoc, opts)
	out, err := Apply(WithLedger(context.Background(), l), f, res, opts, "")
	if err != nil {
		t.Fatal(err)
	}
	u := LedgerChanges("stk_1", 1, res, out)
	if len(u.Claim) != 1 || u.Claim[0].Key != "Node/worker-1" || u.Claim[0].LiveID != "n1" {
		t.Fatalf("claim: %+v", u.Claim)
	}

	owned := NewInstallLedger("stk_1", "nodes-eu", u.Claim)
	popts := Options{Source: "nodes-eu", Prune: true}
	pres := []*Resource{}
	st, err := Load(WithLedger(context.Background(), owned), f, pres, popts)
	if err != nil {
		t.Fatal(err)
	}
	p = PlanFor(st, pres, popts)
	if len(p.Changes) != 1 || p.Changes[0].Action != ActionDelete || !strings.Contains(p.Changes[0].Reason, "never pruned") {
		t.Fatalf("prune: %+v", p.Changes)
	}
	pout, err := Apply(WithLedger(context.Background(), owned), f, pres, popts, "")
	if err != nil {
		t.Fatal(err)
	}
	if pout.Results[0].Status != StatusNoop || len(LedgerChanges("stk_1", 2, pres, pout).Release) != 1 {
		t.Fatalf("prune result: %+v", pout.Results)
	}

	// A rename is detected through the node id.
	g := newFake()
	addNodes(g, "worker-9")
	p = installPlan(t, g, owned, nodeDoc, opts)
	if e := errorsByKey(p)["Node/worker-1"]; !strings.Contains(e, "renamed to worker-9") {
		t.Fatalf("rename: %+v", p.Changes)
	}
}
