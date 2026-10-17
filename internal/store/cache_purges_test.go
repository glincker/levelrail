package store

import (
	"context"
	"testing"
)

func TestCachePurgeHistory(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	t.Setenv(EnvCachePurgeKeep, "2")
	for i, scope := range []string{"url", "prefix", "all"} {
		if err := db.AddCachePurge(ctx, CachePurge{App: "web", Domain: "a.test", Scope: scope, Removed: i, Actor: "root"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.AddCachePurge(ctx, CachePurge{App: "api", Domain: "b.test", Scope: "tag", Value: "x"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListCachePurges(ctx, "web", 0)
	if err != nil || len(got) != 2 || got[0].Scope != "all" || got[1].Scope != "prefix" {
		t.Fatalf("web history = %+v, %v", got, err)
	}
	if all, _ := db.ListCachePurges(ctx, "", 0); len(all) != 3 {
		t.Fatalf("all history = %d", len(all))
	}
}
