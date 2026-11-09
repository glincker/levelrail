package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestTagListings_FollowResourceVisibility(t *testing.T) {
	rt, db := newTestRouter(t)
	ctx := t.Context()
	seedTagApp(t, db, "web")
	seedTagApp(t, db, "secret-app")
	for _, n := range []string{"main", "secret-db"} {
		if err := db.SaveDesiredDatabase(ctx, store.DesiredDatabase{Name: n, Engine: "postgres", Version: "16"}); err != nil {
			t.Fatal(err)
		}
	}
	tag := store.Tag{ID: "tag_scope", Name: "prod"}
	if err := db.SaveTag(ctx, tag); err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"web", "secret-app"} {
		if err := db.AttachAppTag(ctx, tag.ID, a); err != nil {
			t.Fatal(err)
		}
	}
	for _, d := range []string{"main", "secret-db"} {
		if err := db.AttachResourceTag(ctx, tag.ID, store.ResourceRef{Kind: store.ResourceKindDatabase, Name: d}); err != nil {
			t.Fatal(err)
		}
	}
	reader := seedMatrixToken(t, db, "reader", []string{AbilityRead})
	attachTestPolicy(t, db, "deny-app", "Deny", "*", "app:secret-app", store.PrincipalTypeToken, "reader")
	attachTestPolicy(t, db, "deny-db", "Deny", "*", "database:secret-db", store.PrincipalTypeToken, "reader")

	names := func(path string) []string {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		bearer(reader)(req)
		rec := httptest.NewRecorder()
		rt.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", path, rec.Code, rec.Body.String())
		}
		var rows []tagAppResource
		_ = json.Unmarshal(rec.Body.Bytes(), &rows)
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = r.Name
		}
		return out
	}
	if got := names("/api/v1/tags/" + tag.ID + "/apps"); len(got) != 1 || got[0] != "web" {
		t.Fatalf("apps by tag = %v, want only web", got)
	}
	if got := names("/api/v1/tags/" + tag.ID + "/databases"); len(got) != 1 || got[0] != "main" {
		t.Fatalf("databases by tag = %v, want only main", got)
	}
	if !tenantRoutes["GET /api/v1/tags/{id}/databases"] {
		t.Fatal("databases by tag must stay open to team-only members, it filters by visibility")
	}
}

func TestResourceTagCap_IsAtomic(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	ref := store.ResourceRef{Kind: store.ResourceKindDatabase, Name: "main"}
	for i, id := range []string{"tag_a", "tag_b"} {
		if err := db.SaveTag(ctx, store.Tag{ID: id, Name: []string{"a", "b"}[i]}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AttachResourceTagCapped(ctx, "tag_a", ref, 1); err != nil {
		t.Fatalf("first attach: %v", err)
	}
	if err := db.AttachResourceTagCapped(ctx, "tag_a", ref, 1); err != nil {
		t.Fatalf("re-attach at cap must be a no-op: %v", err)
	}
	if err := db.AttachResourceTagCapped(ctx, "tag_b", ref, 1); !errors.Is(err, store.ErrResourceTagCap) {
		t.Fatalf("over cap = %v, want ErrResourceTagCap", err)
	}
}
