package store

import (
	"context"
	"testing"
	"time"
)

func TestPipelineSyncRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	got, err := db.GetPipelineSync(ctx, "web")
	if err != nil || got.AppName != "web" || got.RepoIsTruth || !got.LastSyncAt.IsZero() {
		t.Fatalf("empty state = %+v err %v", got, err)
	}

	at := time.Now().UTC().Truncate(time.Millisecond)
	want := PipelineSync{AppName: "web", RepoIsTruth: true, LastSHA: "abc123", LastSyncAt: at, LastError: "boom"}
	if err := db.SavePipelineSync(ctx, want); err != nil {
		t.Fatal(err)
	}
	want.LastError = ""
	if err := db.SavePipelineSync(ctx, want); err != nil {
		t.Fatal(err)
	}
	got, err = db.GetPipelineSync(ctx, "web")
	if err != nil || !got.RepoIsTruth || got.LastSHA != "abc123" || !got.LastSyncAt.Equal(at) || got.LastError != "" {
		t.Fatalf("saved state = %+v err %v", got, err)
	}
}

func TestSavePipelineKeepsSyncBookkeeping(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()
	p, err := db.SavePipeline(ctx, Pipeline{ID: "p1", AppName: "web", Name: "ci", Source: "repo", YAML: "a", Enabled: true, CreatedAt: now, UpdatedAt: now, SourceSHA: "s1", SyncedHash: "h1"})
	if err != nil {
		t.Fatal(err)
	}
	if p.Source != "repo" || p.SourceSHA != "s1" || p.SyncedHash != "h1" {
		t.Fatalf("saved = %+v", p)
	}
	p.YAML = "edited"
	p, err = db.SavePipeline(ctx, p)
	if err != nil || p.YAML != "edited" || p.SyncedHash != "h1" {
		t.Fatalf("after edit = %+v err %v", p, err)
	}
}
