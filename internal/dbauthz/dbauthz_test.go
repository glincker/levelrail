package dbauthz

import (
	"context"
	"errors"
	"slices"
	"testing"
)

// fakeEngine models flat abilities, explicit denies and explicit allows.
type fakeEngine struct {
	denyActions  []string
	allowActions []string
	hidden       bool
	err          error
}

func (f fakeEngine) Evaluate(_ context.Context, c Caller, ability string, action Action, _ string) (Verdict, error) {
	switch {
	case f.err != nil:
		return Verdict{}, f.err
	case f.hidden:
		return Verdict{Hidden: true}, nil
	case slices.Contains(f.denyActions, string(action)) || slices.Contains(f.denyActions, ability):
		return Verdict{Denied: true}, nil
	case slices.Contains(c.Abilities, ability) || slices.Contains(c.Abilities, "root"):
		return Verdict{Allowed: true}, nil
	case slices.Contains(f.allowActions, string(action)):
		return Verdict{Allowed: true, ByAction: true}, nil
	}
	return Verdict{}, nil
}

func resolve(t *testing.T, r *Resolver, c Caller, res string, a Action) Decision {
	t.Helper()
	d, _ := r.Resolve(context.Background(), c, res, a)
	return d
}

func TestResolveAuthzAllowsAndDenies(t *testing.T) {
	const res = "database:main"
	admin := Caller{Abilities: []string{"root"}}
	viewer := Caller{Abilities: []string{"read"}}
	reader := Caller{Abilities: []string{"read:sensitive"}}
	ciToken := Caller{PrincipalType: "token", Abilities: []string{"deploy"}}
	tests := []struct {
		name   string
		eng    fakeEngine
		caller Caller
		action Action
		want   bool
		reason string
	}{
		{"admin writes", fakeEngine{}, admin, ActionWrite, true, ReasonOK},
		{"reader reads", fakeEngine{}, reader, ActionRead, true, ReasonOK},
		{"reader queries", fakeEngine{}, reader, ActionQuery, true, ReasonOK},
		{"reader cannot write", fakeEngine{}, reader, ActionWrite, false, ReasonNoAbility},
		{"viewer cannot read rows", fakeEngine{}, viewer, ActionRead, false, ReasonNoAbility},
		{"ci token with narrow ability", fakeEngine{}, ciToken, ActionQuery, false, ReasonNoAbility},
		{"policy deny beats root", fakeEngine{denyActions: []string{"db:read"}}, admin, ActionRead, false, ReasonExplicitDeny},
		{"policy allow grants db action", fakeEngine{allowActions: []string{"db:export"}}, viewer, ActionExport, true, ReasonOK},
		{"team scoped caller hidden", fakeEngine{hidden: true}, reader, ActionRead, false, ReasonHidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := resolve(t, &Resolver{Engine: tt.eng}, tt.caller, res, tt.action)
			if d.Allowed != tt.want || d.Reason != tt.reason {
				t.Fatalf("got allowed=%v reason=%q, want %v %q", d.Allowed, d.Reason, tt.want, tt.reason)
			}
		})
	}
}

func TestResolveAuthzFailsClosed(t *testing.T) {
	admin := Caller{Abilities: []string{"root"}}
	if d, err := (&Resolver{Engine: fakeEngine{err: errors.New("boom")}}).Resolve(context.Background(), admin, "database:x", ActionRead); d.Allowed || err == nil {
		t.Fatal("engine error must deny and surface the error")
	}
	if d, err := (&Resolver{}).Resolve(context.Background(), admin, "database:x", ActionRead); d.Allowed || err == nil {
		t.Fatal("missing engine must deny")
	}
	if d, _ := (&Resolver{Engine: fakeEngine{}}).Resolve(context.Background(), admin, "database:x", Action("db:nope")); d.Allowed {
		t.Fatal("unknown action must deny")
	}
	for _, res := range []string{"", "app:x", "database:"} {
		if d, _ := (&Resolver{Engine: fakeEngine{}}).Resolve(context.Background(), admin, res, ActionRead); d.Allowed {
			t.Fatalf("resource %q must deny", res)
		}
	}
}

func TestResolveAuthzStepUp(t *testing.T) {
	r := &Resolver{Engine: fakeEngine{}, EnforceStepUp: true}
	admin := Caller{Abilities: []string{"root"}}
	if d := resolve(t, r, admin, "database:x", ActionWrite); d.Allowed || !d.RequiredStepUp || d.Reason != ReasonStepUp {
		t.Fatalf("expired step up must deny a write: %+v", d)
	}
	admin.StepUpFresh = true
	if d := resolve(t, r, admin, "database:x", ActionWrite); !d.Allowed {
		t.Fatalf("fresh step up must allow: %+v", d)
	}
	admin.StepUpFresh = false
	if d := resolve(t, r, admin, "database:x", ActionRead); !d.Allowed {
		t.Fatal("reads need no step up")
	}
}

func TestEffectiveAbilityMapping(t *testing.T) {
	if EffectiveAbility(ActionRead) != "read:sensitive" || EffectiveAbility(ActionWrite) != "root" || EffectiveAbility(Action("x")) != "root" {
		t.Fatal("effective ability mapping drifted")
	}
}

func TestDenyNamesImplication(t *testing.T) {
	if got := DenyNames(ActionExport); len(got) != 3 || got[1] != "db:query" || got[2] != "db:read" {
		t.Fatalf("export deny names = %v", got)
	}
	if got := DenyNames(ActionQuery); len(got) != 2 || got[1] != "db:read" {
		t.Fatalf("query deny names = %v", got)
	}
	if got := DenyNames(ActionWrite); len(got) != 1 {
		t.Fatalf("write deny names = %v", got)
	}
}
