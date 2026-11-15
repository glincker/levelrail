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

func TestWriteJSONKeepsUnexportedEmbeddedFields(t *testing.T) {
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, embeddedHolder{embeddedBase: embeddedBase{ID: "x"}})
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got["id"] != "x" {
		t.Fatalf("promoted field lost: %s", rec.Body.String())
	}
	for _, k := range []string{"items", "tags"} {
		if arr, ok := got[k].([]any); !ok || len(arr) != 0 {
			t.Fatalf("%s should be an empty array: %s", k, rec.Body.String())
		}
	}
}
