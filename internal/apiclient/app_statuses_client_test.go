package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListAppStatuses(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"name":"web","image":"x","status":{"label":"Attention needed","variant":"destructive"}}]`))
	}))
	defer srv.Close()

	got, err := NewClient(srv.URL, "test-token").ListAppStatuses(context.Background())
	if err != nil {
		t.Fatalf("ListAppStatuses() error = %v", err)
	}
	if gotMethod != http.MethodGet || gotPath != "/api/v1/apps" {
		t.Errorf("request = %s %s, want GET /api/v1/apps", gotMethod, gotPath)
	}
	if len(got) != 1 || got[0].Name != "web" || got[0].Status.Variant != "destructive" || got[0].Status.Label != "Attention needed" {
		t.Errorf("ListAppStatuses() = %+v", got)
	}
}
