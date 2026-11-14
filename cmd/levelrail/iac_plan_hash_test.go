package main

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/secrets"
)

// emptyDoer answers every read with an empty body: a control plane with nothing in it.
type emptyDoer struct{}

func (emptyDoer) Do(context.Context, string, string, any, any) error { return nil }

func planHashAfterBoot(t *testing.T, serialized string) string {
	t.Helper()
	mk, err := secrets.LoadMasterKey(serialized)
	if err != nil {
		t.Fatalf("LoadMasterKey: %v", err)
	}
	setupIaCPlanHashKey(secrets.NewManager(nil, mk))
	res := []*iac.Resource{{Kind: iac.KindProject, Name: "p", Fields: map[string]any{"description": "d"}, Doc: &iac.Document{File: "a.yaml", Line: 1}}}
	opts := iac.Options{Source: "ci"}
	st, err := iac.Load(context.Background(), emptyDoer{}, res, opts)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return iac.PlanFor(st, res, opts).Hash
}

func TestIaCPlanHashStableAcrossServerConstructions(t *testing.T) {
	mk, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	other, err := secrets.GenerateMasterKey()
	if err != nil {
		t.Fatal(err)
	}
	a, b := planHashAfterBoot(t, mk.String()), planHashAfterBoot(t, mk.String())
	if a != b {
		t.Fatalf("same master key gave %q then %q", a, b)
	}
	if c := planHashAfterBoot(t, other.String()); c == a {
		t.Fatal("a different master key gave the same hash")
	}
}
