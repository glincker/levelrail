package models

import (
	"context"
	"errors"
	"testing"
)

func TestService_CreateWithSwapGroup(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(t, nil)

	if _, err := svc.Create(ctx, CreateInput{Spec: spec("bad"), SwapGroup: "Not Valid!"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad swap group err = %v, want ErrInvalid", err)
	}

	got, err := svc.Create(ctx, CreateInput{Spec: spec("a"), SwapGroup: "gpu0"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Model.SwapGroup != "gpu0" {
		t.Fatalf("SwapGroup = %q, want gpu0", got.Model.SwapGroup)
	}
}

func TestService_SetSwapGroup(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(t, nil)
	if _, err := svc.Create(ctx, CreateInput{Spec: spec("a")}); err != nil {
		t.Fatal(err)
	}

	if err := svc.SetSwapGroup(ctx, "a", "gpu0"); err != nil {
		t.Fatalf("SetSwapGroup: %v", err)
	}
	v, err := svc.Get(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if v.Model.SwapGroup != "gpu0" {
		t.Fatalf("SwapGroup = %q, want gpu0", v.Model.SwapGroup)
	}

	if err := svc.SetSwapGroup(ctx, "a", "Not Valid!"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad swap group err = %v, want ErrInvalid", err)
	}
}

func TestService_SharesGPUWith(t *testing.T) {
	ctx := context.Background()
	svc, _ := newSvc(t, nil)
	if _, err := svc.Create(ctx, CreateInput{Spec: spec("a"), SwapGroup: "gpu0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, CreateInput{Spec: spec("b"), SwapGroup: "gpu0"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, CreateInput{Spec: spec("c")}); err != nil {
		t.Fatal(err)
	}

	a, err := svc.Get(ctx, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(a.SharesGPUWith) != 1 || a.SharesGPUWith[0] != "b" {
		t.Fatalf("a.SharesGPUWith = %v, want [b]", a.SharesGPUWith)
	}

	c, err := svc.Get(ctx, "c")
	if err != nil {
		t.Fatal(err)
	}
	if len(c.SharesGPUWith) != 0 {
		t.Fatalf("c.SharesGPUWith = %v, want none (no group)", c.SharesGPUWith)
	}

	list, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string][]string{}
	for _, v := range list {
		byName[v.Model.Name] = v.SharesGPUWith
	}
	if len(byName["a"]) != 1 || byName["a"][0] != "b" {
		t.Fatalf("List a.SharesGPUWith = %v, want [b]", byName["a"])
	}
	if len(byName["b"]) != 1 || byName["b"][0] != "a" {
		t.Fatalf("List b.SharesGPUWith = %v, want [a]", byName["b"])
	}
}
