package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRun_TagsList(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []tagResource{
		{ID: "tag_1", Name: "production", CreatedAt: "2026-09-20T00:00:00Z"},
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"tags", "list", "--api-url", srv.URL})
	if gotPath != "/api/v1/tags" {
		t.Errorf("path = %q, want /api/v1/tags", gotPath)
	}
	if !strings.Contains(stdout, "tag_1") {
		t.Errorf("stdout = %q, want the tag id listed", stdout)
	}
}

func TestRun_TagsCreate(t *testing.T) {
	runCreateTagLikeSuccess(t,
		[]string{"tags", "create", "--name", "production"},
		"/api/v1/tags", "production",
	)
}

func TestRun_TagsCreate_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"tags", "create"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
}

func TestRun_TagsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newTagResolveThenServer(t, "production", "tag_1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	_, _ = runCLIExpectOK(t, []string{"tags", "delete", "production", "--api-url", srv.URL})
	if *gotPath != "/api/v1/tags/tag_1" {
		t.Errorf("path = %q, want /api/v1/tags/tag_1", *gotPath)
	}
	if *gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", *gotMethod)
	}
}

func TestRun_TagsDelete_NameNotFound(t *testing.T) {
	srv := newListEchoServer(t, nil, []tagResource{{ID: "tag_1", Name: "production", CreatedAt: "2026-09-20T00:00:00Z"}})
	defer srv.Close()

	stderr := runCLIExpectAPIError(t, []string{"tags", "delete", "staging", "--api-url", srv.URL})
	if !strings.Contains(stderr, `tag "staging" not found`) {
		t.Errorf("stderr = %q, want a tag-not-found error", stderr)
	}
}

func TestRun_TagsApps(t *testing.T) {
	srv, gotPath, _ := newTagResolveThenServer(t, "production", "tag_1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]tagAppResource{{Name: "web"}, {Name: "worker"}})
	})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"tags", "apps", "production", "--api-url", srv.URL})
	if *gotPath != "/api/v1/tags/tag_1/apps" {
		t.Errorf("path = %q, want /api/v1/tags/tag_1/apps", *gotPath)
	}
	if !strings.Contains(stdout, "web") || !strings.Contains(stdout, "worker") {
		t.Errorf("stdout = %q, want both app names listed", stdout)
	}
}

func TestRun_AppsTag(t *testing.T) {
	runCreateTagLikeSuccess(t,
		[]string{"apps", "tag", "web", "production"},
		"/api/v1/apps/web/tags", "production",
	)
}

func TestRun_AppsTag_RequiresTwoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "tag", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
}

func TestRun_AppsUntag(t *testing.T) {
	srv, gotPath, gotMethod := newTagResolveThenServer(t, "production", "tag_1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	_, _ = runCLIExpectOK(t, []string{"apps", "untag", "web", "production", "--api-url", srv.URL})
	if *gotPath != "/api/v1/apps/web/tags/tag_1" {
		t.Errorf("path = %q, want /api/v1/apps/web/tags/tag_1", *gotPath)
	}
	if *gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", *gotMethod)
	}
}

func TestRun_AppsUntag_NameNotFound(t *testing.T) {
	srv := newListEchoServer(t, nil, []tagResource{{ID: "tag_1", Name: "production", CreatedAt: "2026-09-20T00:00:00Z"}})
	defer srv.Close()

	stderr := runCLIExpectAPIError(t, []string{"apps", "untag", "web", "staging", "--api-url", srv.URL})
	if !strings.Contains(stderr, `tag "staging" not found`) {
		t.Errorf("stderr = %q, want a tag-not-found error", stderr)
	}
}
