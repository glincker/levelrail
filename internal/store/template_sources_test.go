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
	if err := db.ReplaceTemplateSourceEntries(ctx, "acme", entries); err != nil {
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
