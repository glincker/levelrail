package githubapp

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

func TestListIssueComments_PagesUntilShort(t *testing.T) {
	var pages []string
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		pages = append(pages, r.URL.Query().Get("page"))
		if r.URL.Path != "/repos/acme/widgets/issues/42/comments" {
			t.Errorf("path = %q", r.URL.Path)
		}
		batch := make([]commentResponse, 0, 100)
		if r.URL.Query().Get("page") == "1" {
			for i := 1; i <= 100; i++ {
				batch = append(batch, commentResponse{ID: int64(i), Body: "c"})
			}
		} else {
			batch = append(batch, commentResponse{ID: 101, Body: "last"})
		}
		_ = json.NewEncoder(w).Encode(batch)
	})

	got, err := c.ListIssueComments(context.Background(), "", "tok", "acme", "widgets", 42)
	if err != nil {
		t.Fatalf("ListIssueComments() error = %v", err)
	}
	if len(got) != 101 || got[100].Body != "last" {
		t.Errorf("got %d comments, want 101 ending in last", len(got))
	}
	if len(pages) != 2 {
		t.Errorf("pages requested = %v, want 2", pages)
	}
}

func TestUpdateIssueComment_PatchesBody(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody createIssueCommentRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(http.StatusOK)
	})

	if err := c.UpdateIssueComment(context.Background(), "", "tok", "acme", "widgets", 77, "new"); err != nil {
		t.Fatalf("UpdateIssueComment() error = %v", err)
	}
	if gotMethod != http.MethodPatch || gotPath != "/repos/acme/widgets/issues/comments/77" || gotBody.Body != "new" {
		t.Errorf("got %s %s %+v", gotMethod, gotPath, gotBody)
	}
}
