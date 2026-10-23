package store

import (
	"testing"
	"time"
)

func TestGitSourceReliabilityTracksWebhookPollAndHead(t *testing.T) {
	db := openTestDB(t)
	ctx := t.Context()
	if err := db.SetGitSourceWebhookState(ctx, "ghost", GitWebhookFailed, "x", time.Now()); err != nil {
		t.Fatalf("a source that does not exist must not create a row: %v", err)
	}
	if r, err := db.GetGitSourceReliability(ctx, "ghost"); err != nil || r.WebhookState != "" {
		t.Fatalf("ghost = %+v, %v", r, err)
	}

	if err := db.SaveGitSource(ctx, GitSource{ServiceName: "web", RepoURL: "https://github.com/org/web.git", Branch: "main", BuildType: "dockerfile"}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	if err := db.SetGitSourceWebhookState(ctx, "web", GitWebhookFailed, "permission denied", at); err != nil {
		t.Fatal(err)
	}
	if err := db.NoteGitSourceHead(ctx, "web", "sha1", at.Add(time.Minute), false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitSourcePollResult(ctx, "web", at.Add(2*time.Minute), "unreachable"); err != nil {
		t.Fatal(err)
	}
	r, err := db.GetGitSourceReliability(ctx, "web")
	if err != nil || r.WebhookState != GitWebhookFailed || r.WebhookError != "permission denied" || r.LastSeenHead != "sha1" || r.PollError != "unreachable" || !r.LastWebhookAt.IsZero() {
		t.Fatalf("r = %+v, %v", r, err)
	}

	if err := db.NoteGitSourceHead(ctx, "web", "sha2", at.Add(3*time.Minute), true); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGitSourceWebhookState(ctx, "web", GitWebhookRegistered, "", at.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	r, _ = db.GetGitSourceReliability(ctx, "web")
	if r.LastSeenHead != "sha2" || r.LastWebhookAt.IsZero() || r.WebhookState != GitWebhookRegistered || r.WebhookError != "" {
		t.Fatalf("r = %+v", r)
	}

	if err := db.DeleteGitSource(ctx, "web"); err != nil {
		t.Fatal(err)
	}
	if r, _ = db.GetGitSourceReliability(ctx, "web"); r.LastSeenHead != "" {
		t.Fatalf("bookkeeping survived the git source: %+v", r)
	}
}
