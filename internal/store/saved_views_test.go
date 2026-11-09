package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSavedViewsVisibility(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	for _, v := range []SavedView{
		{ID: "a1", Owner: "user:a", TeamID: "team-a", List: "apps", Name: "mine", Filter: `{"q":"x"}`, Shared: SavedViewPrivate},
		{ID: "a2", Owner: "user:a", TeamID: "team-a", List: "apps", Name: "shared", Filter: `{}`, Shared: SavedViewTeam},
		{ID: "b1", Owner: "user:b", TeamID: "team-b", List: "apps", Name: "theirs", Filter: `{}`, Shared: SavedViewTeam},
		{ID: "c1", Owner: "user:c", TeamID: "team-a", List: "apps", Name: "private", Filter: `{}`, Shared: SavedViewPrivate},
	} {
		if _, err := db.UpsertSavedView(ctx, v, 10); err != nil {
			t.Fatal(err)
		}
	}
	got, err := db.ListSavedViews(ctx, SavedViewFilter{Owner: "user:c", Teams: []string{"team-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "c1" || got[1].ID != "a2" {
		t.Fatalf("team a member sees %+v", got)
	}
	got, _ = db.ListSavedViews(ctx, SavedViewFilter{Owner: "user:x", Teams: []string{"team-c"}})
	if len(got) != 0 {
		t.Fatalf("outsider sees %+v", got)
	}
	again, err := db.UpsertSavedView(ctx, SavedView{ID: "new", Owner: "user:a", TeamID: "team-a", List: "apps", Name: "mine", Filter: `{"q":"y"}`, Shared: SavedViewTeam}, 2)
	if err != nil || again.ID != "a1" || again.Filter != `{"q":"y"}` {
		t.Fatalf("upsert kept %+v %v", again, err)
	}
	if n, _ := db.CountSavedViews(ctx, "user:a"); n != 2 {
		t.Fatalf("count %d", n)
	}
	if _, err := db.UpsertSavedView(ctx, SavedView{ID: "a3", Owner: "user:a", TeamID: "team-a", List: "apps", Name: "third", Filter: `{}`, Shared: SavedViewPrivate}, 2); !errors.Is(err, ErrSavedViewLimit) {
		t.Fatalf("over the cap: %v", err)
	}
	if err := db.DeleteSavedView(ctx, "a1"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSavedView(ctx, "a1"); !errors.Is(err, ErrSavedViewNotFound) {
		t.Fatalf("get after delete %v", err)
	}
}

func TestSavedViewsDroppedWithOwner(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveAPIToken(ctx, APIToken{ID: "tok-v", Name: "tok-v", TokenHash: "h-tok-v"}); err != nil {
		t.Fatal(err)
	}
	for _, v := range []SavedView{
		{ID: "t1", Owner: "token:tok-v", List: "apps", Name: "t", Filter: `{}`, Shared: SavedViewPrivate},
		{ID: "u1", Owner: "user:gone", List: "apps", Name: "u", Filter: `{}`, Shared: SavedViewPrivate},
	} {
		if _, err := db.UpsertSavedView(ctx, v, 10); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.RevokeAPIToken(ctx, "tok-v"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSavedView(ctx, "t1"); !errors.Is(err, ErrSavedViewNotFound) {
		t.Fatalf("token view kept after revoke: %v", err)
	}
	if err := db.CreateUser(ctx, User{ID: "gone", Email: "gone@example.test", DisplayName: "G", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteUser(ctx, "gone"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetSavedView(ctx, "u1"); !errors.Is(err, ErrSavedViewNotFound) {
		t.Fatalf("user view kept after delete: %v", err)
	}
}
