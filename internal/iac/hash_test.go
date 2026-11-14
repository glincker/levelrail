package iac

import (
	"context"
	"errors"
	"strings"
	"testing"
)

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

func TestPlanHashV1Accepted(t *testing.T) {
	SetPlanHashKey([]byte("k"))
	f := newFake()
	res := mustBuild(t, fixture, Options{})
	st, _ := Load(context.Background(), f, res, Options{})
	p := PlanFor(st, res, Options{})
	on := true
	SetV1HashAcceptedSource(func() bool { return on })
	t.Cleanup(func() { SetV1HashAcceptedSource(nil) })

	if err := CheckPlanHash(p.Hash, p); err != nil {
		t.Fatalf("v2: %v", err)
	}
	if err := CheckPlanHash(PlanHashV1(p), p); err != nil {
		t.Fatalf("v1 accepted: %v", err)
	}
	if err := CheckPlanHash("0123456789abcdef", p); !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("wrong v1 = %v", err)
	}
	if err := CheckPlanHash("v2:"+strings.Repeat("0", 64), p); !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("wrong v2 = %v", err)
	}
	on = false
	if err := CheckPlanHash(PlanHashV1(p), p); !errors.Is(err, ErrPlanHashVersion) {
		t.Fatalf("v1 refused = %v", err)
	}
	if _, err := Apply(context.Background(), f, res, Options{}, PlanHashV1(p)); !errors.Is(err, ErrPlanHashVersion) {
		t.Fatalf("apply v1 refused = %v", err)
	}
}
