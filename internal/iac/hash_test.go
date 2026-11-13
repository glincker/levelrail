package iac

import "testing"

func v2Plan(t *testing.T, mutate func(*Resource)) Plan {
	t.Helper()
	res := []*Resource{{Kind: KindProject, Name: "p", Fields: map[string]any{"description": "d"}, Doc: &Document{File: "a.yaml", Line: 1}}}
	if mutate != nil {
		mutate(res[0])
	}
	return PlanFor(&State{projects: map[string]wireProject{}}, res, Options{Source: "ci"})
}

func TestPlanHashV2(t *testing.T) {
	SetPlanHashKey([]byte("test-key-one"))
	base := v2Plan(t, nil)
	if len(base.Hash) < 10 || base.Hash[:3] != HashV2Prefix {
		t.Fatalf("hash %q lacks v2 prefix", base.Hash)
	}
	moved := v2Plan(t, func(r *Resource) {
		r.Doc = &Document{File: "other.yaml", Line: 40}
		r.Warnings = []string{"reworded warning"}
	})
	if moved.Hash != base.Hash {
		t.Fatal("moving a document or rewording a warning changed the hash")
	}
	changed := v2Plan(t, func(r *Resource) { r.Fields = map[string]any{"description": "other"} })
	if changed.Hash == base.Hash {
		t.Fatal("changing a desired value did not change the hash")
	}
	SetPlanHashKey([]byte("test-key-two"))
	if v2Plan(t, nil).Hash == base.Hash {
		t.Fatal("a different key gave the same hash")
	}
	if PlanHashV1(base) == "" {
		t.Fatal("v1 hash unavailable")
	}
}
