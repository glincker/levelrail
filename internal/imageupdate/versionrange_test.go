package imageupdate

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewestInRange(t *testing.T) {
	tags := []string{"latest", "1.24.9", "1.25.0", "1.25.3", "1.25.4-rc1", "1.25.5-alpine", "1.26.0-beta.1", "2.0.0", "v1.9.0", "1.25.10"}
	cases := []struct{ spec, current, want string }{
		{"1.x", "1.25.0", "1.25.10"},
		{"1.25.x", "1.25.0", "1.25.10"},
		{"~1.24.0", "1.24.1", "1.24.9"},
		{"^1.2.3", "1.25.0", "1.25.10"},
		{">=1.0.0 <1.25.0", "1.9.0", "1.24.9"},
		{"1.x", "1.25.0-alpine", "1.25.5-alpine"},
		{"*", "1.0.0", "2.0.0"},
	}
	for _, c := range cases {
		r, err := ParseRange(c.spec)
		if err != nil {
			t.Fatalf("%q: %v", c.spec, err)
		}
		got, ok := NewestInRange(tags, r, c.current)
		if !ok || got != c.want {
			t.Errorf("%q from %q: got %q ok=%v, want %q", c.spec, c.current, got, ok, c.want)
		}
	}
	if _, err := ParseRange("banana"); err == nil {
		t.Error("ParseRange(banana) should fail")
	}
	if IsNewer("1.25.0", "1.25.10") || !IsNewer("1.25.10", "latest") || IsNewer("1.25.0", "1.25.0") {
		t.Error("IsNewer ordering is wrong")
	}
}

func TestHTTPTagListerFollowsPages(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("last") == "" {
			w.Header().Set("Link", `</v2/team/app/tags/list?last=1.0.0>; rel="next"`)
			_, _ = w.Write([]byte(`{"tags":["1.0.0"]}`))
			return
		}
		_, _ = w.Write([]byte(`{"tags":["1.1.0"]}`))
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "https://")
	tags, err := HTTPTagLister{Client: srv.Client()}.ListTags(context.Background(), host+"/team/app:1.0.0", nil)
	if err != nil || len(tags) != 2 {
		t.Fatalf("tags = %v, err = %v; want both pages", tags, err)
	}
}
