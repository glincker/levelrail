package store

import (
	"context"
	"testing"
	"time"
)

func TestCertLedgerRoundTripAndPrune(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	for _, e := range []CertLedgerEntry{
		{"a.dev", "issued", now.Add(-time.Hour)},
		{"a.dev", "issued", now.Add(-time.Hour)},
		{"b.dev", "failed", now.Add(-30 * 24 * time.Hour)},
	} {
		if err := db.SaveCertLedger(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.LoadCertLedger(ctx, now, 7*24*time.Hour)
	if err != nil || len(got) != 1 || got[0].Domain != "a.dev" {
		t.Fatalf("got %+v, err %v", got, err)
	}
}
