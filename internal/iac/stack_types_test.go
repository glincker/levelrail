package iac

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestValidateStackName(t *testing.T) {
	for _, ok := range []string{"shop-prod", "a", "s3"} {
		if err := ValidateStackName(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "Shop", "1shop", "shop_prod", "shop.prod", strings.Repeat("a", maxStackNameLen+1)} {
		if ValidateStackName(bad) == nil {
			t.Errorf("%q: want error", bad)
		}
	}
}

func TestParseDriftPolicy(t *testing.T) {
	if p, err := ParseDriftPolicy(""); err != nil || p != DriftWarn {
		t.Fatalf("default = %q, %v", p, err)
	}
	if p, err := ParseDriftPolicy("lock"); err != nil || p != DriftLock {
		t.Fatalf("lock = %q, %v", p, err)
	}
	if _, err := ParseDriftPolicy("revert"); err == nil {
		t.Fatal("revert must be rejected")
	}
	spec := StackSpec{Project: "shop"}
	if err := spec.Validate(); err != nil || spec.Drift != DriftWarn {
		t.Fatalf("spec default drift = %q, %v", spec.Drift, err)
	}
	if (&StackSpec{}).Validate() == nil {
		t.Fatal("spec without project must fail")
	}
}

func TestStackPruneAndPin(t *testing.T) {
	s := Stack{Name: "shop", ProjectID: "p1", DriftPolicy: DriftWarn}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(s.PruneAllowed(true), ErrPruneNotAllowed) {
		t.Fatal("manifest alone must not grant prune")
	}
	s.AllowPrune = true
	if s.PruneAllowed(false) == nil || s.PruneAllowed(true) != nil {
		t.Fatal("prune needs both sides")
	}
	if s.CheckPin(0) != nil {
		t.Fatal("unpinned stack must accept applies")
	}
	s.PinnedRevision = 3
	if !errors.Is(s.CheckPin(0), ErrStackPinned) || !errors.Is(s.CheckPin(2), ErrStackPinned) {
		t.Fatal("pinned stack must refuse other applies")
	}
	if s.CheckPin(3) != nil {
		t.Fatal("rollback to the pin must pass")
	}
}

func TestRevisionStatusTransitions(t *testing.T) {
	cases := []struct {
		from, to RevisionStatus
		ok       bool
	}{
		{RevisionPendingApproval, RevisionApplying, true},
		{RevisionApplying, RevisionPartial, true},
		{RevisionApplied, RevisionSuperseded, true},
		{RevisionApplied, RevisionApplying, false},
		{RevisionFailed, RevisionApplied, false},
		{RevisionSuperseded, RevisionApplied, false},
	}
	for _, c := range cases {
		if got := c.from.CanTransition(c.to); got != c.ok {
			t.Errorf("%s -> %s = %v, want %v", c.from, c.to, got, c.ok)
		}
	}
	if RevisionStatus("done").Valid() || !RevisionApplying.Valid() || RevisionApplying.Terminal() {
		t.Fatal("status validity or terminality wrong")
	}
	r := Revision{StackID: "s1", Number: 3, Status: RevisionApplying, RollbackOf: 1}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.RollbackOf = 3
	if r.Validate() == nil {
		t.Fatal("rollback_of must be earlier")
	}
}

func TestLeakedSecret(t *testing.T) {
	docs := []byte("env:\n  DB_URL: { secretRef: DB_URL }\n  MODE: prod\n")
	if _, leaked := LeakedSecret(docs, map[string]string{"DB_URL": "s3cr3t-value-for-test", "EMPTY": ""}); leaked {
		t.Fatal("secret names alone must not count as a leak")
	}
	name, leaked := LeakedSecret(append(docs, "X: s3cr3t-value-for-test\n"...), map[string]string{"DB_URL": "s3cr3t-value-for-test"})
	if !leaked || name != "DB_URL" {
		t.Fatalf("leak not caught: %q %v", name, leaked)
	}
}

func TestStackScopeContextAndLease(t *testing.T) {
	ctx := context.Background()
	if _, ok := StackScopeFrom(ctx); ok {
		t.Fatal("empty context must carry no scope")
	}
	ctx = WithStackScope(ctx, StackScope{StackID: "s1", StackName: "shop", Revision: 4})
	if s, ok := StackScopeFrom(ctx); !ok || s.Revision != 4 {
		t.Fatalf("scope = %+v, %v", s, ok)
	}
	now := time.Now()
	l := StackLock{ExpiresAt: now.Add(time.Minute)}
	if l.Expired(now) || !l.Expired(now.Add(time.Minute)) {
		t.Fatal("lease expiry wrong")
	}
	if m, err := ParseStackDeleteMode(""); err != nil || m != StackForget {
		t.Fatalf("delete default = %q, %v", m, err)
	}
	if _, err := ParseStackDeleteMode("purge"); err == nil {
		t.Fatal("unknown delete mode must fail")
	}
}
