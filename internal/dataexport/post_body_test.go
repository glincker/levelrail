package dataexport

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPostBodyDropsEmptyHeaders(t *testing.T) {
	t.Parallel()
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
	}))
	defer srv.Close()
	hdr := map[string]string{"X-Empty": "", "X-Set": "v"}
	if err := postBody(context.Background(), srv.Client(), srv.URL, "application/json", hdr, []byte("{}"), "t"); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["X-Empty"]; ok || got.Get("X-Set") != "v" {
		t.Fatalf("headers = %v", got)
	}
}
