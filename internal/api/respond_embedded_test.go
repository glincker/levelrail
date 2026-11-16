package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

type embeddedBase struct {
	ID    string   `json:"id"`
	Items []string `json:"items"`
}

type embeddedHolder struct {
	embeddedBase
	Tags []string `json:"tags"`
}

func TestWriteJSONLeavesUnexportedEmbeddedUnchanged(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, embeddedHolder{embeddedBase: embeddedBase{ID: "x"}})
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "x" {
		t.Fatalf("promoted field lost: %s", rec.Body.String())
	}
	// Nil slices inside the unexported embedded struct are left as they are.
	if v, ok := got["items"]; ok && v != nil {
		t.Fatalf("embedded items should stay null: %s", rec.Body.String())
	}
	if arr, ok := got["tags"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("tags should be an empty array: %s", rec.Body.String())
	}
}
