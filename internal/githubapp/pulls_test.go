package githubapp

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

func TestGetPullRequest_ParsesHeadAndBase(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/acme/widgets/pulls/7" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"number":7,"state":"open","head":{"ref":"feat","sha":"abc","repo":{"full_name":"acme/widgets"}},"base":{"ref":"main","repo":{"full_name":"acme/widgets"}}}`))
	})
	pr, err := c.GetPullRequest(context.Background(), "", "tok", "acme", "widgets", 7)
	if err != nil {
		t.Fatalf("GetPullRequest() error = %v", err)
	}
	if pr.State != "open" || pr.HeadSHA != "abc" || pr.HeadRepoFullName != "acme/widgets" || pr.BaseRef != "main" {
		t.Errorf("pr = %+v", pr)
	}
}

func TestGetPullRequest_DeletedHeadRepo(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"number":7,"state":"open","head":{"ref":"feat","sha":"abc","repo":null},"base":{"ref":"main","repo":{"full_name":"acme/widgets"}}}`))
	})
	pr, err := c.GetPullRequest(context.Background(), "", "tok", "acme", "widgets", 7)
	if err != nil || pr.HeadRepoFullName != "" {
		t.Fatalf("pr = %+v, err = %v, want empty head repo", pr, err)
	}
}

func TestGetCollaboratorPermission(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/collaborators/octo/permission") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"permission":"write"}`))
	})
	got, err := c.GetCollaboratorPermission(context.Background(), "", "tok", "acme", "widgets", "octo")
	if err != nil || got != "write" {
		t.Fatalf("got %q, err %v", got, err)
	}
}

func TestGetCollaboratorPermission_ErrorPropagates(t *testing.T) {
	c := newTestClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })
	if _, err := c.GetCollaboratorPermission(context.Background(), "", "tok", "acme", "widgets", "octo"); err == nil {
		t.Fatal("want an error on 403")
	}
}
