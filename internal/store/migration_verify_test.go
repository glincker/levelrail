package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMigrationVerifyReportReplacesPreviousRow(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if _, err := db.GetMigrationVerifyReport(ctx); !errors.Is(err, ErrMigrationVerifyNotFound) {
		t.Fatalf("empty store: err = %v", err)
	}
	at := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	for _, v := range []string{"fail", "pass"} {
		if err := db.SaveMigrationVerifyReport(ctx, MigrationVerifyReport{Platform: "coolify", GeneratedAt: at, Verdict: v, Body: []byte(`{"verdict":"` + v + `"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.GetMigrationVerifyReport(ctx)
	if err != nil || got.Verdict != "pass" || string(got.Body) != `{"verdict":"pass"}` || !got.GeneratedAt.Equal(at) {
		t.Fatalf("got %+v, err %v", got, err)
	}
}
