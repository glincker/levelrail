package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRepoReachabilityNoteUnreachable(t *testing.T) {
	inf := upInference{Flags: createFlags{repo: "file:///nonexistent/levelrail-repo"}}
	note := repoReachabilityNote(context.Background(), t.TempDir(), "cli", inf)
	if !strings.Contains(note, "may be private") || !strings.Contains(note, "cli deploy") {
		t.Fatalf("expected a private-repo note with the upload path, got %q", note)
	}
}

func TestRepoReachabilityNoteNoRepo(t *testing.T) {
	if note := repoReachabilityNote(context.Background(), t.TempDir(), "cli", upInference{}); note != "" {
		t.Fatalf("no repo means no note, got %q", note)
	}
}

func TestBuildProgressLabel(t *testing.T) {
	if got := buildProgressLabel(0); got != "building" {
		t.Fatalf("got %q", got)
	}
	if got := buildProgressLabel(130 * time.Second); got != "building (last build took 2m10s)" {
		t.Fatalf("got %q", got)
	}
}
