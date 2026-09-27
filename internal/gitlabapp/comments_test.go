package gitlabapp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListAndUpdateMergeRequestNotes(t *testing.T) {
	var updatedPath, updatedMethod string
	var updatedBody createNoteRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			if r.URL.EscapedPath() != "/api/v4/projects/org%2Fweb/merge_requests/42/notes" {
				t.Errorf("list path = %q", r.URL.EscapedPath())
			}
			_ = json.NewEncoder(w).Encode([]noteResponse{{ID: 1, Body: "a"}, {ID: 2, Body: "b"}})
		default:
			updatedMethod, updatedPath = r.Method, r.URL.EscapedPath()
			_ = json.NewDecoder(r.Body).Decode(&updatedBody)
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	c := &Client{HTTP: srv.Client()}
	got, err := c.ListMergeRequestNotes(context.Background(), srv.URL, "at", "org/web", 42)
	if err != nil {
		t.Fatalf("ListMergeRequestNotes() error = %v", err)
	}
	if len(got) != 2 || got[1].ID != 2 || got[1].Body != "b" {
		t.Errorf("notes = %+v", got)
	}

	if err := c.UpdateMergeRequestNote(context.Background(), srv.URL, "at", "org/web", 42, 2, "new"); err != nil {
		t.Fatalf("UpdateMergeRequestNote() error = %v", err)
	}
	if updatedMethod != http.MethodPut || updatedPath != "/api/v4/projects/org%2Fweb/merge_requests/42/notes/2" || updatedBody.Body != "new" {
		t.Errorf("update = %s %s %+v", updatedMethod, updatedPath, updatedBody)
	}
}
