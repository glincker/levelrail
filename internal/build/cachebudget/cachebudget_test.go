package cachebudget

import "testing"

func TestBudgetForUnknownDiskUsesFixedBudget(t *testing.T) {
	if b := BudgetFor(0, 0, 0); b.MaxUsedBytes != 5120<<20 || b.ReservedBytes != 2048<<20 {
		t.Fatalf("unknown disk budget = %+v", b)
	}
}

func TestSizeClampsReservedToMaxUsed(t *testing.T) {
	s := Size(500<<30, 1024, 4096, 0)
	if s.ReservedBytes != s.MaxUsedBytes || s.MinFreePct != 15 {
		t.Fatalf("sizing = %+v", s)
	}
}
