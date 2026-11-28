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

func TestTemplateSourceReviewStates(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.CreateTemplateSource(ctx, TemplateSource{Name: "acme", GitURL: "https://git.example.com/a.git"}); err != nil {
		t.Fatal(err)
	}
	mk := func(id, hash string) TemplateSourceEntry {
		return TemplateSourceEntry{Source: "acme", ID: id, Name: id, ContentHash: hash}
	}
	steps := []struct {
		name    string
		entries []TemplateSourceEntry
		approve [2]string
		want    map[string]string
	}{
		{"new id needs review", []TemplateSourceEntry{mk("acme/app", "h1")}, [2]string{}, map[string]string{"acme/app": TemplateReviewNeedsReview}},
		{"approve h1", nil, [2]string{"acme/app", "h1"}, map[string]string{"acme/app": TemplateReviewApproved}},
		{"same hash stays approved", []TemplateSourceEntry{mk("acme/app", "h1")}, [2]string{}, map[string]string{"acme/app": TemplateReviewApproved}},
		{"rename is a new id", []TemplateSourceEntry{mk("acme/other", "h1")}, [2]string{}, map[string]string{"acme/other": TemplateReviewNeedsReview}},
		{"dropped", []TemplateSourceEntry{}, [2]string{}, map[string]string{}},
		{"readd same hash stays approved", []TemplateSourceEntry{mk("acme/app", "h1")}, [2]string{}, map[string]string{"acme/app": TemplateReviewApproved}},
		{"dropped again", []TemplateSourceEntry{}, [2]string{}, map[string]string{}},
		{"readd different hash needs review", []TemplateSourceEntry{mk("acme/app", "h2")}, [2]string{}, map[string]string{"acme/app": TemplateReviewNeedsReview}},
	}
	for _, st := range steps {
		if st.approve[0] != "" {
			if err := db.ApproveTemplateSourceEntry(ctx, "acme", st.approve[0], st.approve[1]); err != nil {
				t.Fatalf("%s: %v", st.name, err)
			}
		} else if err := db.ReplaceTemplateSourceEntries(ctx, "acme", "sha", st.entries); err != nil {
			t.Fatal(err)
		}
		rows, err := db.ListTemplateSourceEntries(ctx, "acme")
		if err != nil || len(rows) != len(st.want) {
			t.Fatalf("%s: %+v, %v", st.name, rows, err)
		}
		for _, r := range rows {
			if r.Review != st.want[r.ID] {
				t.Fatalf("%s: %s is %s", st.name, r.ID, r.Review)
			}
		}
	}
	if got, _ := db.GetTemplateSource(ctx, "acme"); got.CommitSHA != "sha" {
		t.Fatalf("commit not stored: %+v", got)
	}
}

func TestTemplateSourceApproveHashMismatch(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.CreateTemplateSource(ctx, TemplateSource{Name: "acme", GitURL: "https://git.example.com/a.git"}); err != nil {
		t.Fatal(err)
	}
	e := TemplateSourceEntry{Source: "acme", ID: "acme/app", Name: "App", ContentHash: "h2"}
	if err := db.ReplaceTemplateSourceEntries(ctx, "acme", "sha", []TemplateSourceEntry{e}); err != nil {
		t.Fatal(err)
	}
	if err := db.ApproveTemplateSourceEntry(ctx, "acme", "acme/app", "h1"); !errors.Is(err, ErrTemplateSourceHashMismatch) {
		t.Fatalf("stale hash: %v", err)
	}
	if err := db.ApproveTemplateSourceEntry(ctx, "acme", "acme/none", "h2"); !errors.Is(err, ErrTemplateSourceNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	rows, _ := db.ListTemplateSourceEntries(ctx, "acme")
	if rows[0].Review != TemplateReviewNeedsReview {
		t.Fatalf("mismatch approved the row: %+v", rows[0])
	}
}
