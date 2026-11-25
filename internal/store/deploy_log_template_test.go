package store

import (
	"context"
	"testing"
)

func TestDeployLogTemplatesRoundTrip(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	rows := []DeployLogTemplate{
		{TemplateID: "a", Template: "boom <NUM>", Count: 2, Sample: "boom 1"},
		{TemplateID: "b", Template: "oom killed", Count: 9, Sample: "oom killed"},
	}
	if err := db.ReplaceDeployLogTemplates(ctx, "d1", rows); err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceDeployLogTemplates(ctx, "d1", rows); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListDeployLogTemplates(ctx, "d1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].TemplateID != "b" || got[0].Count != 9 {
		t.Fatalf("got %+v", got)
	}
}
