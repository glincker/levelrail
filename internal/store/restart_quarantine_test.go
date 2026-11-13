package store

import (
	"context"
	"testing"
	"time"
)

func TestRestartQuarantineDroppedWithAppAndPruned(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	at := time.Now()
	for _, q := range []RestartQuarantine{
		{Container: "web-abcd1234", AppName: "web", Restarts: 3, QuarantinedAt: at},
		{Container: "gone-abcd1234", AppName: "gone", Restarts: 3, QuarantinedAt: at},
	} {
		if err := db.SaveRestartQuarantine(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := db.PruneRestartQuarantine(ctx); err != nil || n != 1 {
		t.Fatalf("prune n=%d err=%v, want the orphan row only", n, err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListRestartQuarantine(ctx)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%v err=%v, want none after app delete", rows, err)
	}
}
