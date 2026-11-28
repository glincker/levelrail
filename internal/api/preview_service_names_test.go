package api

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/glincker/levelrail/internal/spec"
	"github.com/glincker/levelrail/internal/store"
	"github.com/glincker/levelrail/internal/webhook"
)

func TestPreviewMultiServiceRefusesForeignName(t *testing.T) {
	rt, db, _, _ := setUpPreviewApp(t)
	ctx := context.Background()
	if err := db.SaveApp(ctx, store.App{ID: "shop-pr-3-api", Name: "shop-pr-3-api", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "shop-pr-3-api", AppID: "shop-pr-3-api", Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	gs := store.GitSource{Services: map[string]spec.Service{"api": {}, "web": {}}}
	ev := webhook.PullRequestEvent{Number: 3, HeadRef: "feat", HeadSHA: "abc123"}
	row, done, status, msg := rt.beginPreview(ctx, "shop", gs, ev, false)
	if row != nil || !done || status != http.StatusConflict {
		t.Fatalf("preview onto another app's service name = %d %q, want 409 and no row", status, msg)
	}
	if _, err := rt.lockAdmitPreviewServices(ctx, "shop-pr-3", []string{"api"}); !errors.Is(err, errAppNameTaken) {
		t.Fatalf("lockAdmit err = %v, want taken", err)
	}
}

func TestPreviewShapedAppNamesRefusedForNewApps(t *testing.T) {
	rt, _ := newTestRouter(t)
	ctx := context.Background()
	for _, n := range []string{"web-pr-1", "web-pr-12-api"} {
		if err := rt.appNameFree(ctx, n); !errors.Is(err, errAppNameTaken) {
			t.Fatalf("%s: err = %v, want taken", n, err)
		}
		if err := rt.appNameFree(withPreviewSystem(ctx), n); err != nil {
			t.Fatalf("%s from the preview system: err = %v", n, err)
		}
	}
	if err := rt.appNameFree(ctx, "pr-review"); err != nil {
		t.Fatalf("plain name: err = %v", err)
	}
}
