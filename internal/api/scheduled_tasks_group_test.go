package api

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestJobAppNotFoundMessage_NamesGroupServices(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := context.Background()
	if err := db.SaveApp(ctx, store.App{ID: "grp", Name: "grp", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatalf("seed group: %v", err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "grp-web", Image: "x/web:1", Port: 80}); err != nil {
		t.Fatalf("seed service: %v", err)
	}
	if err := db.UpdateServiceApp(ctx, "grp-web", "grp"); err != nil {
		t.Fatalf("link service: %v", err)
	}
	if got := rt.jobAppNotFoundMessage(ctx, "grp"); !strings.Contains(got, "grp-web") {
		t.Fatalf("message = %q, want it to name grp-web", got)
	}
	if got := rt.jobAppNotFoundMessage(ctx, "missing"); got != "app not found" {
		t.Fatalf("message = %q, want plain not found", got)
	}
}
