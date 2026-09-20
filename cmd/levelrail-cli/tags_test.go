package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	var gotPath, gotMethod string
	var gotBody createTagRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(tagResource{ID: "tag_1", Name: gotBody.Name, CreatedAt: "2026-09-20T00:00:00Z"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"tags", "create", "--name", "production", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/tags" {
		t.Errorf("path = %q, want /api/v1/tags", gotPath)
	}
	if gotBody.Name != "production" {
		t.Errorf("request body = %+v, want name production", gotBody)
	}
}

func TestRun_TagsCreate_MissingName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"tags", "create"}, &stdout, &stderr, envMap())
	if got != exitValidation {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitValidation, stderr.String())
	}
}

func TestRun_TagsDelete(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	_, _ = runCLIExpectOK(t, []string{"tags", "delete", "tag_1", "--api-url", srv.URL})
	if *gotPath != "/api/v1/tags/tag_1" {
		t.Errorf("path = %q, want /api/v1/tags/tag_1", *gotPath)
	}
	if *gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", *gotMethod)
	}
}

func TestRun_TagsApps(t *testing.T) {
	var gotPath string
	srv := newListEchoServer(t, &gotPath, []tagAppResource{{Name: "web"}, {Name: "worker"}})
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"tags", "apps", "tag_1", "--api-url", srv.URL})
	if gotPath != "/api/v1/tags/tag_1/apps" {
		t.Errorf("path = %q, want /api/v1/tags/tag_1/apps", gotPath)
	}
	if !strings.Contains(stdout, "web") || !strings.Contains(stdout, "worker") {
		t.Errorf("stdout = %q, want both app names listed", stdout)
	}
}

func TestRun_AppsTag(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody attachAppTagRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(tagResource{ID: "tag_1", Name: gotBody.Name, CreatedAt: "2026-09-20T00:00:00Z"})
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{
		"apps", "tag", "web", "production", "--api-url", srv.URL,
	}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, want %d (stdout=%q stderr=%q)", got, exitOK, stdout.String(), stderr.String())
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/apps/web/tags" {
		t.Errorf("path = %q, want /api/v1/apps/web/tags", gotPath)
	}
	if gotBody.Name != "production" {
		t.Errorf("request body = %+v, want name production", gotBody)
	}
}

func TestRun_AppsTag_RequiresTwoArgs(t *testing.T) {
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"apps", "tag", "web"}, &stdout, &stderr, envMap())
	if got != exitUsage {
		t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitUsage, stderr.String())
	}
}

func TestRun_AppsUntag(t *testing.T) {
	srv, gotPath, gotMethod := newNoContentEchoServer(t)
	defer srv.Close()

	_, _ = runCLIExpectOK(t, []string{"apps", "untag", "web", "tag_1", "--api-url", srv.URL})
	if *gotPath != "/api/v1/apps/web/tags/tag_1" {
		t.Errorf("path = %q, want /api/v1/apps/web/tags/tag_1", *gotPath)
	}
	if *gotMethod != http.MethodDelete {
		t.Errorf("method = %q, want DELETE", *gotMethod)
	}
}
