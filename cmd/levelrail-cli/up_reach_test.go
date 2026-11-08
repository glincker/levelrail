package main

import (
	"context"
	"os/exec"
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

func TestRepoReachabilityNoteHidesCredentials(t *testing.T) {
	inf := upInference{Flags: createFlags{repo: "https://user:TOKEN123@127.0.0.1:1/repo.git"}} //nolint:gosec // fake token for the redaction test
	note := repoReachabilityNote(context.Background(), t.TempDir(), "cli", inf)
	if note == "" || strings.Contains(note, "TOKEN123") || strings.Contains(note, "user:") {
		t.Fatalf("note must be set and carry no credentials, got %q", note)
	}
}

func TestRepoReachabilityNoteEmptyRepoIsNotPrivate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := t.TempDir()
	if out, err := exec.Command("git", "init", "--bare", "-q", bare).CombinedOutput(); err != nil { //nolint:gosec // fixed args, temp dir
		t.Fatalf("git init: %v %s", err, out)
	}
	inf := upInference{Flags: createFlags{repo: "file://" + bare}}
	note := repoReachabilityNote(context.Background(), t.TempDir(), "cli", inf)
	if strings.Contains(note, "may be private") || !strings.Contains(note, "no branches") {
		t.Fatalf("empty reachable repo mislabeled: %q", note)
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
