package store

import (
	"context"
	"errors"
	"testing"
)

func TestTemplateSourcesLifecycle(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	src := TemplateSource{Name: "acme", GitURL: "https://git.example.com/acme/t.git"}
	if err := db.CreateTemplateSource(ctx, src); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateTemplateSource(ctx, src); !errors.Is(err, ErrTemplateSourceExists) {
		t.Fatalf("duplicate: %v", err)
	}
	entries := []TemplateSourceEntry{{Source: "acme", ID: "acme/app", Name: "App", TemplateYAML: "a", ComposeYAML: "b"}}
	if err := db.ReplaceTemplateSourceEntries(ctx, "acme", "abc", entries); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTemplateSourceStatus(ctx, "acme", "ok", ""); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetTemplateSource(ctx, "acme")
	if err != nil || got.Templates != 1 || got.Status != "ok" || got.LastSync == "" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if err := db.DeleteTemplateSource(ctx, "acme"); err != nil {
		t.Fatal(err)
	}
	if rows, _ := db.ListTemplateSourceEntries(ctx, ""); len(rows) != 0 {
		t.Fatalf("entries survived delete: %+v", rows)
	}
	if err := db.DeleteTemplateSource(ctx, "acme"); !errors.Is(err, ErrTemplateSourceNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestTemplateSourceHashChangeNeedsReview(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.CreateTemplateSource(ctx, TemplateSource{Name: "acme", GitURL: "https://git.example.com/a.git"}); err != nil {
		t.Fatal(err)
	}
	e := TemplateSourceEntry{Source: "acme", ID: "acme/app", Name: "App"}
	steps := []struct {
		name, hash, want string
		approve          bool
	}{
		{"first sync is approved", "h1", TemplateReviewApproved, false},
		{"same hash stays approved", "h1", TemplateReviewApproved, false},
		{"changed hash needs review", "h2", TemplateReviewNeedsReview, false},
		{"stays flagged while unreviewed", "h2", TemplateReviewNeedsReview, false},
		{"approval clears it", "h2", TemplateReviewApproved, true},
	}
	for _, st := range steps {
		e.ContentHash = st.hash
		if st.approve {
			if err := db.ApproveTemplateSourceEntry(ctx, "acme", "acme/app"); err != nil {
				t.Fatal(err)
			}
		} else if err := db.ReplaceTemplateSourceEntries(ctx, "acme", "sha", []TemplateSourceEntry{e}); err != nil {
			t.Fatal(err)
		}
		rows, err := db.ListTemplateSourceEntries(ctx, "acme")
		if err != nil || len(rows) != 1 || rows[0].Review != st.want {
			t.Fatalf("%s: %+v, %v", st.name, rows, err)
		}
	}
	if got, _ := db.GetTemplateSource(ctx, "acme"); got.CommitSHA != "sha" {
		t.Fatalf("commit not stored: %+v", got)
	}
}
