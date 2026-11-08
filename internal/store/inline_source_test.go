package store

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestInlineSource_SaveGetDedupeAndCascade(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "web:1", Port: 8080}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	src := InlineSource{ServiceName: "web", Dockerfile: "FROM scratch\nCOPY a.txt /\n", Files: []InlineFile{{Path: "a.txt", Content: "hi"}}}
	hash, err := db.SaveInlineSource(ctx, src)
	if err != nil || len(hash) != 64 {
		t.Fatalf("save = %q, %v", hash, err)
	}
	if again, err := db.SaveInlineSource(ctx, src); err != nil || again != hash {
		t.Fatalf("second save = %q, %v, want the same hash", again, err)
	}
	got, err := db.GetInlineSource(ctx, "web", hash)
	if err != nil || got.Dockerfile != src.Dockerfile || len(got.Files) != 1 || got.Files[0].Content != "hi" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	if got.SizeBytes != InlineSourceSize(src.Dockerfile, src.Files) {
		t.Errorf("size = %d", got.SizeBytes)
	}
	if _, err := db.GetInlineSource(ctx, "other", hash); !errors.Is(err, ErrInlineSourceNotFound) {
		t.Errorf("other app get err = %v, want not found", err)
	}
	if err := db.DeleteDesiredService(ctx, "web"); err != nil {
		t.Fatalf("delete app: %v", err)
	}
	if _, err := db.GetInlineSource(ctx, "web", hash); !errors.Is(err, ErrInlineSourceNotFound) {
		t.Errorf("after app delete err = %v, want cascade", err)
	}
}

func TestPruneInlineSources_KeepsNewestAndInFlight(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "web", Image: "web:1", Port: 8080}); err != nil {
		t.Fatalf("seed app: %v", err)
	}
	var hashes []string
	for _, df := range []string{"FROM a", "FROM b", "FROM c", "FROM d"} {
		h, err := db.SaveInlineSource(ctx, InlineSource{ServiceName: "web", Dockerfile: df})
		if err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, h)
		time.Sleep(2 * time.Millisecond)
	}
	if err := db.SaveDeployAttempt(ctx, DeployAttempt{ID: "att_q", ServiceName: "web", Image: "web:x", Status: DeployAttemptStatusQueued, CommitSHA: hashes[0][:40], StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDeployAttempt(ctx, DeployAttempt{ID: "att_done", ServiceName: "web", Image: "web:x", Status: DeployAttemptStatusSucceeded, CommitSHA: hashes[1][:40], StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	n, err := db.PruneInlineSources(ctx, 1)
	if err != nil || n != 2 {
		t.Fatalf("pruned %d, %v, want 2", n, err)
	}
	for i, want := range []bool{true, false, false, true} {
		_, err := db.GetInlineSource(ctx, "web", hashes[i])
		if got := err == nil; got != want {
			t.Errorf("source %d kept = %v, want %v (%v)", i, got, want, err)
		}
	}
}

func TestInlineSourceHash_LengthPrefixed(t *testing.T) {
	a := InlineSourceHash("FROM a", []InlineFile{{Path: "bc", Content: ""}})
	b := InlineSourceHash("FROM ab", []InlineFile{{Path: "c", Content: ""}})
	if a == b {
		t.Fatal("different splits of the same bytes hashed the same")
	}
}
