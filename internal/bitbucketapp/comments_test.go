package bitbucketapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListAndUpdatePullRequestComments(t *testing.T) {
	var updatedPath, updatedMethod string
	var updatedBody createPRCommentRequest
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("page") == "":
			_ = json.NewEncoder(w).Encode(prCommentPage{
				Values: []prCommentResponse{{ID: 1, Content: createPRCommentContent{Raw: "a"}}},
				Next:   srvURL + r.URL.Path + "?page=2",
			})
		case r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(prCommentPage{Values: []prCommentResponse{{ID: 2, Content: createPRCommentContent{Raw: "b"}}}})
		default:
			updatedMethod, updatedPath = r.Method, r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&updatedBody)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL

	c := &Client{HTTP: srv.Client(), APIBaseURL: srv.URL}
	got, err := c.ListPullRequestComments(context.Background(), "tok", "acme/widgets", 42)
	if err != nil {
		t.Fatalf("ListPullRequestComments() error = %v", err)
	}
	if len(got) != 2 || got[1].ID != 2 || got[1].Body != "b" {
		t.Errorf("comments = %+v", got)
	}

	if err := c.UpdatePullRequestComment(context.Background(), "tok", "acme/widgets", 42, 2, "new"); err != nil {
		t.Fatalf("UpdatePullRequestComment() error = %v", err)
	}
	if updatedMethod != http.MethodPut || updatedPath != "/repositories/acme/widgets/pullrequests/42/comments/2" || updatedBody.Content.Raw != "new" {
		t.Errorf("update = %s %s %+v", updatedMethod, updatedPath, updatedBody)
	}
}
