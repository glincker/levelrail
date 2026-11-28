package store

import (
	"context"
	"errors"
	"testing"
)

// The env merge lands between the snapshot read and the first save, so the
// revision check must reject that save and the retry must keep every env key.
func TestDesiredServiceSnapshotSaveInterleavedWithEnvMerge(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "x:1", Port: 80, Env: map[string]string{"A": "1"}}); err != nil {
		t.Fatal(err)
	}
	snap, err := db.GetDesiredService(ctx, "web")
	if err != nil {
		t.Fatal(err)
	}
	var attempts, stale int
	save := func(ctx context.Context, svc DesiredService) error {
		attempts++
		if attempts == 1 {
			if _, err := db.MergeServiceEnv(ctx, "web", map[string]string{"B": "2", "C": "3"}, nil); err != nil {
				t.Fatalf("merge: %v", err)
			}
		}
		err := db.SaveDesiredService(ctx, svc)
		if errors.Is(err, ErrStaleDesiredService) {
			stale++
		}
		return err
	}
	build := func(base DesiredService) DesiredService { base.Image = "x:2"; return base }
	if _, err := SaveRebased(ctx, build(*snap), db.GetDesiredService, save, build); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || stale != 1 {
		t.Fatalf("attempts = %d, stale rejections = %d, want 2 and 1", attempts, stale)
	}
	got, _ := db.GetDesiredService(ctx, "web")
	if got.Image != "x:2" {
		t.Fatalf("image = %q, want x:2", got.Image)
	}
	for k, v := range map[string]string{"A": "1", "B": "2", "C": "3"} {
		if got.Env[k] != v {
			t.Fatalf("env %s = %q, want %q (env %v)", k, got.Env[k], v, got.Env)
		}
	}
}
