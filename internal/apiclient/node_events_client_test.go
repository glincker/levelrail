package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListNodeEvents(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"from_status":"online","to_status":"offline","created_at":"2026-09-23T10:00:00Z"}]`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "t")
	got, err := c.ListNodeEvents(context.Background(), "nd_1", 0)
	if err != nil {
		t.Fatalf("ListNodeEvents() error = %v", err)
	}
	if gotPath != "/api/v1/nodes/nd_1/events" || gotQuery != "" {
		t.Errorf("request = %s?%s", gotPath, gotQuery)
	}
	if len(got) != 1 || got[0].ToStatus != "offline" {
		t.Errorf("got %+v", got)
	}
	if _, err := c.ListNodeEvents(context.Background(), "nd_1", 7); err != nil || gotQuery != "limit=7" {
		t.Errorf("limit query = %q, err = %v", gotQuery, err)
	}
}
