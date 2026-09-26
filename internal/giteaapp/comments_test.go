package giteaapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListAndUpdateIssueComments(t *testing.T) {
	var updatedPath, updatedMethod string
	var updatedBody createIssueCommentRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/api/v1/repos/acme/widgets/issues/42/comments" {
				t.Errorf("list path = %q", r.URL.Path)
			}
			_ = json.NewEncoder(w).Encode([]commentResponse{{ID: 1, Body: "a"}, {ID: 2, Body: "b"}})
		default:
			updatedMethod, updatedPath = r.Method, r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&updatedBody)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	got, err := c.ListIssueComments(context.Background(), srv.URL, "tok", "acme/widgets", 42)
	if err != nil {
		t.Fatalf("ListIssueComments() error = %v", err)
	}
	if len(got) != 2 || got[1].ID != 2 || got[1].Body != "b" {
		t.Errorf("comments = %+v", got)
	}

	if err := c.UpdateIssueComment(context.Background(), srv.URL, "tok", "acme/widgets", 2, "new"); err != nil {
		t.Fatalf("UpdateIssueComment() error = %v", err)
	}
	if updatedMethod != http.MethodPatch || updatedPath != "/api/v1/repos/acme/widgets/issues/comments/2" || updatedBody.Body != "new" {
		t.Errorf("update = %s %s %+v", updatedMethod, updatedPath, updatedBody)
	}
}
