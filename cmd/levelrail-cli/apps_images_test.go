package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestRun_AppsImages(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []imageResource{
		{Tag: "abc123", CreatedAt: time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)},
		{Tag: "def456", CreatedAt: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "images", "web", "--api-url", srv.URL})
	if gotPath != "/api/v1/apps/web/images" {
		t.Errorf("path = %q, want /api/v1/apps/web/images", gotPath)
	}
	if !strings.Contains(stdout, "abc123") || !strings.Contains(stdout, "def456") {
		t.Errorf("stdout = %q, want both tags listed", stdout)
	}
}

func TestRun_AppsImages_Empty(t *testing.T) {
	srv := newListEchoServer(t, nil, []imageResource{})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "images", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "no images") {
		t.Errorf("stdout = %q, want the empty-list message", stdout)
	}
}

func TestRun_AppsImages_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "images"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d", got, exitUsage)
	}
	if !strings.Contains(stderr.String(), "requires exactly one") {
		t.Errorf("stderr = %q, want a missing-name usage error", stderr.String())
	}
}

func TestRun_AppsImages_NotFound(t *testing.T) {
	srv := newJSONErrorServer(t, 404, `{"error":"app not found"}`)

	stderr := runCLIExpectAPIError(t, []string{"apps", "images", "ghost", "--api-url", srv.URL})
	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr = %q, want the server's not-found message", stderr)
	}
}

func TestRun_AppsImages_Help(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "images", "-h"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d", got, exitOK)
	}
	if !strings.Contains(stderr.String(), "apps images") {
		t.Errorf("stderr = %q, want usage text", stderr.String())
	}
}
