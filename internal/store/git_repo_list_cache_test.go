package store

import (
	"errors"
	"testing"
	"time"
)

func TestGitRepoListCacheRoundTripAndPrefixDelete(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	for _, k := range []string{"github:1", "github:2", "gitlab"} {
		if err := db.PutGitRepoListCache(ctx, GitRepoListCacheEntry{Key: k, Provider: "p", Payload: `{"repos":[]}`, Meta: `["e"]`, Truncated: k == "gitlab", FetchedAt: at}); err != nil {
			t.Fatalf("put %s: %v", k, err)
		}
	}
	got, err := db.GetGitRepoListCache(ctx, "gitlab")
	if err != nil || !got.Truncated || got.Meta != `["e"]` || !got.FetchedAt.Equal(at) {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if err := db.PutGitRepoListCache(ctx, GitRepoListCacheEntry{Key: "gitlab", Provider: "p", Payload: `{"repos":[1]}`, FetchedAt: at}); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if got, _ = db.GetGitRepoListCache(ctx, "gitlab"); got.Truncated || got.Payload != `{"repos":[1]}` {
		t.Fatalf("overwrite did not replace the row: %+v", got)
	}
	if err := db.DeleteGitRepoListCachePrefix(ctx, "github"); err != nil {
		t.Fatalf("delete prefix: %v", err)
	}
	if _, err := db.GetGitRepoListCache(ctx, "github:1"); !errors.Is(err, ErrGitRepoListCacheMiss) {
		t.Fatalf("github:1 err = %v, want a miss", err)
	}
	if _, err := db.GetGitRepoListCache(ctx, "gitlab"); err != nil {
		t.Fatalf("gitlab must survive a github prefix delete: %v", err)
	}
}

func TestGitConnectionStatusRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	if _, ok, err := db.GetGitConnectionStatus(ctx, "gitlab"); err != nil || ok {
		t.Fatalf("empty get = ok %v, err %v", ok, err)
	}
	now := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	if err := db.SetGitConnectionStatus(ctx, GitConnectionStatus{Provider: "gitlab", Status: GitConnNeedsReauthorize, Detail: "grant revoked", NextCheckAt: now.Add(time.Minute), UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetGitConnectionStatus(ctx, "gitlab")
	if err != nil || !ok || got.Status != GitConnNeedsReauthorize || got.Detail != "grant revoked" || !got.NextCheckAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("get = %+v, %v, %v", got, ok, err)
	}
	if err := db.DeleteGitConnectionStatus(ctx, "gitlab"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ = db.GetGitConnectionStatus(ctx, "gitlab"); ok {
		t.Fatal("status survived delete")
	}
}
