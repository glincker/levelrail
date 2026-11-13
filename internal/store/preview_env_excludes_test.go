package store

import (
	"context"
	"errors"
	"testing"
)

func TestPreviewEnvExcludes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "img:v1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"B", "A", "A"} {
		if err := db.SetPreviewEnvExclude(ctx, "web", k, true); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := db.ListPreviewEnvExcludes(ctx, "web")
	if len(got) != 2 || got[0] != "A" {
		t.Fatalf("excludes = %v, want [A B]", got)
	}
	if err := db.SetPreviewEnvExclude(ctx, "web", "A", false); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.ListPreviewEnvExcludes(ctx, "web"); len(got) != 1 {
		t.Fatalf("after include = %v", got)
	}
	if err := db.SetPreviewEnvExclude(ctx, "nope", "A", true); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("missing app err = %v", err)
	}
}
