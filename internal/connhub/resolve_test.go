package connhub

import "testing"

func TestResolveOrgDefaultInheritedAndAppOverride(t *testing.T) {
	conns := map[string]Connection{
		"registry:org": {Ref: "registry:org", ScopeType: "org", ScopeID: "acme"},
		"registry:app": {Ref: "registry:app", ScopeType: "instance"},
		"notify:org":   {Ref: "notify:org", ScopeType: "org", ScopeID: "acme"},
	}
	target := Target{Type: "app", ID: "web", TeamID: "acme"}
	org := []Binding{
		{ID: "o1", Ref: "registry:org", TargetType: "org", TargetID: "acme", Role: "registry_pull", Origin: "binding"},
		{ID: "o2", Ref: "notify:org", TargetType: "org", TargetID: "acme", Role: "notify", Origin: "binding"},
	}

	got := Resolve(nil, org, conns, target)
	if len(got) != 2 {
		t.Fatalf("inherited only = %+v", got)
	}
	for _, r := range got {
		if r.Origin != "inherited" || r.InheritedFrom != "acme" || r.Overridden {
			t.Errorf("inherited row = %+v", r)
		}
	}

	app := []Binding{{ID: "a1", Ref: "registry:app", TargetType: "app", TargetID: "web", Role: "registry_pull", Origin: "binding"}}
	got = Resolve(app, org, conns, target)
	if len(got) != 3 {
		t.Fatalf("with override = %+v", got)
	}
	// Roles sort alphabetically, so notify leads; for registry_pull the app row wins and the org row it replaces is marked overridden.
	if got[0].Role != "notify" || got[0].Overridden {
		t.Errorf("notify default must stay effective: %+v", got[0])
	}
	var winner, loser *ResolvedBinding
	for i := range got {
		if got[i].Role != "registry_pull" {
			continue
		}
		if got[i].Overridden {
			loser = &got[i]
		} else {
			winner = &got[i]
		}
	}
	if winner == nil || winner.Ref != "registry:app" || winner.InheritedFrom != "" {
		t.Errorf("winner = %+v", winner)
	}
	if loser == nil || loser.Ref != "registry:org" || loser.InheritedFrom != "acme" {
		t.Errorf("overridden org default = %+v", loser)
	}

	// An org default whose connection scope no longer covers the app is dropped.
	other := Target{Type: "app", ID: "web", TeamID: "beta"}
	if got := Resolve(nil, org, conns, other); len(got) != 0 {
		t.Errorf("org default leaked across orgs: %+v", got)
	}
}
