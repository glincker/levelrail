package store

import (
	"context"
	"testing"
	"time"
)

func TestNodeAgentVersionChangesRecordedOnlyOnChange(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	saveCertNode(t, db, time.Now().Add(time.Hour))
	now := time.Now()
	for _, v := range []string{"v0.9.0", "v0.9.0", "v0.9.1", "v0.9.1", "v0.9.0"} {
		if err := db.UpdateNodeAgentInfo(ctx, "n1", NodeAgentInfo{Version: v}, now); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListNodeAgentVersionChanges(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("changes = %d, want 3: %+v", len(got), got)
	}
	if got[0].FromVersion != "v0.9.1" || got[0].ToVersion != "v0.9.0" || got[2].FromVersion != "" {
		t.Fatalf("changes = %+v", got)
	}
}
