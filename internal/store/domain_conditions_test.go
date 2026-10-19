package store

import (
	"context"
	"testing"
)

func TestDomainConditions_RoundTripAndClear(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if _, _, known, err := db.ListDomainConditions(ctx); err != nil || known {
		t.Fatalf("fresh store: known = %v, err = %v, want unknown", known, err)
	}

	want := []DomainCondition{{Domain: "a.example.com", Code: DomainCondBasicAuthUnresolved, Severity: DomainConditionDropped, Message: "m"}}
	if err := db.ReplaceDomainConditions(ctx, want); err != nil {
		t.Fatalf("ReplaceDomainConditions() error = %v", err)
	}
	got, at, known, err := db.ListDomainConditions(ctx)
	if err != nil || !known || at.IsZero() {
		t.Fatalf("ListDomainConditions() = %v, %v, %v, %v", got, at, known, err)
	}
	if len(got) != 1 || got[0] != want[0] {
		t.Errorf("got %+v, want %+v", got, want)
	}

	if err := db.ReplaceDomainConditions(ctx, nil); err != nil {
		t.Fatalf("clear error = %v", err)
	}
	got, _, known, err = db.ListDomainConditions(ctx)
	if err != nil || !known || len(got) != 0 {
		t.Errorf("after clear: %v, %v, %v, want known and empty", got, known, err)
	}
}
