package telemetry

import (
	"context"
	"strings"
	"testing"
)

func TestPreviewPipeline(t *testing.T) {
	rules := []PipelineRule{
		{Kind: PipelineMultiline, Pattern: `^\d{4}-\d{2}-\d{2}`},
		{Kind: PipelineRedact, Pattern: `password=\S+`, Replace: "password=***"},
		{Kind: PipelineDrop, Pattern: `healthcheck`},
		{Kind: PipelineParse, Pattern: `^(?P<day>\S+) (?P<level>[A-Z]+) `},
	}
	lines := []string{
		"2026-10-15 ERROR boom password=hunter2",
		"\tat main.go:10",
		"2026-10-15 INFO healthcheck ok",
		"2026-10-15 WARN slow",
	}
	got, err := PreviewPipeline(rules, lines)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 records, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Message, "password=***") || !strings.Contains(got[0].Message, "at main.go:10") || strings.Contains(got[0].Message, "hunter2") {
		t.Fatalf("redact/multiline: %q", got[0].Message)
	}
	if got[0].Fields["level"] != "ERROR" {
		t.Fatalf("parse fields: %+v", got[0].Fields)
	}
	if !got[1].Dropped || got[2].Dropped {
		t.Fatalf("drop: %+v", got)
	}
}

func TestValidatePipeline(t *testing.T) {
	cases := map[string][]PipelineRule{
		"unknown kind": {{Kind: "x", Pattern: "a"}},
		"bad regex":    {{Kind: PipelineDrop, Pattern: "("}},
		"empty":        {{Kind: PipelineDrop}},
		"no groups":    {{Kind: PipelineParse, Pattern: `\d+`}},
		"two multi":    {{Kind: PipelineMultiline, Pattern: "a"}, {Kind: PipelineMultiline, Pattern: "b"}},
	}
	for name, rules := range cases {
		if err := ValidatePipeline(rules); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestLogPipelineStoreRoundTrip(t *testing.T) {
	db := newTestDB(t)
	ctx := context.Background()
	rules := []PipelineRule{{Kind: PipelineRedact, Pattern: `secret`}}
	if err := db.SetLogPipeline(ctx, "service:web", rules); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetLogPipeline(ctx, "service:web")
	if err != nil || len(got) != 1 || got[0].Pattern != "secret" {
		t.Fatalf("get: %v %+v", err, got)
	}
	if err := db.SetLogPipeline(ctx, "service:web", nil); err != nil {
		t.Fatal(err)
	}
	if got, _ = db.GetLogPipeline(ctx, "service:web"); got != nil {
		t.Fatalf("cleared pipeline still returned %+v", got)
	}
}
