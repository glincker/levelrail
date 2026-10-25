package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPRCommentCommandSettings_DefaultOffAndIndependent(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveGitSource(ctx, GitSource{ServiceName: "web", RepoURL: "https://example.com/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetGitSource(ctx, "web")
	if err != nil || got.PRCommentCommands || got.PRCommentCommandsForks {
		t.Fatalf("defaults = %+v, err = %v, want both off", got, err)
	}
	if err := db.SetGitSourcePRCommentCommands(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetGitSource(ctx, "web")
	if !got.PRCommentCommands || got.PRCommentCommandsForks {
		t.Fatalf("after enabling commands = %+v, want forks still off", got)
	}
	// An ordinary connect-form save must never move the opt-in.
	if err := db.SaveGitSource(ctx, GitSource{ServiceName: "web", RepoURL: "https://example.com/web.git", Branch: "dev", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetGitSource(ctx, "web")
	if !got.PRCommentCommands {
		t.Error("SaveGitSource reset pr_comment_commands")
	}
	if err := db.SetGitSourcePRCommentCommandsForks(ctx, "web", true); err != nil {
		t.Fatal(err)
	}
	got, _ = db.GetGitSource(ctx, "web")
	if !got.PRCommentCommandsForks {
		t.Error("forks flag not stored")
	}
	if err := db.SetGitSourcePRCommentCommands(ctx, "missing", true); !errors.Is(err, ErrGitSourceNotFound) {
		t.Errorf("err = %v, want ErrGitSourceNotFound", err)
	}
}

func TestClaimPRCommentCommand_FirstWinsAndPrunes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)

	first, err := db.ClaimPRCommentCommand(ctx, "web", 42, "octo", "redeploy", 7, now, time.Hour)
	if err != nil || !first {
		t.Fatalf("first claim = %v, %v", first, err)
	}
	again, err := db.ClaimPRCommentCommand(ctx, "web", 42, "octo", "redeploy", 7, now, time.Hour)
	if err != nil || again {
		t.Fatalf("second claim = %v, %v, want false", again, err)
	}
	other, _ := db.ClaimPRCommentCommand(ctx, "api", 42, "octo", "redeploy", 7, now, time.Hour)
	if !other {
		t.Error("the same comment id for another app must be claimable")
	}
	later, err := db.ClaimPRCommentCommand(ctx, "web", 42, "octo", "redeploy", 7, now.Add(2*time.Hour), time.Hour)
	if err != nil || !later {
		t.Fatalf("claim after retention = %v, %v, want old rows pruned", later, err)
	}
}
