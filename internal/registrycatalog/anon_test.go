package registrycatalog

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListTagsAnonymousTokenAndPaging(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/token":
			if r.URL.Query().Get("scope") != "repository:library/postgres:pull" {
				t.Errorf("scope = %q", r.URL.Query().Get("scope"))
			}
			_, _ = w.Write([]byte(`{"token":"abc"}`))
		case r.Header.Get("Authorization") != "Bearer abc":
			w.Header().Set("WWW-Authenticate", `Bearer realm="`+srv.URL+`/token",service="registry"`)
			w.WriteHeader(http.StatusUnauthorized)
		case r.URL.Query().Get("last") == "":
			w.Header().Set("Link", `</v2/library/postgres/tags/list?n=1000&last=17.6>; rel="next"`)
			_, _ = w.Write([]byte(`{"tags":["17.5","17.6"]}`))
		default:
			_, _ = w.Write([]byte(`{"tags":["17.7"]}`))
		}
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	tags, err := c.ListTagsAnonymous(context.Background(), srv.URL, "library/postgres")
	if err != nil || len(tags) != 3 {
		t.Fatalf("tags = %v, err = %v", tags, err)
	}
}

func TestListTagsAnonymousRateLimit(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "42")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client()}
	_, err := c.ListTagsAnonymous(context.Background(), srv.URL, "library/redis")
	var rl *RateLimitError
	if !errors.As(err, &rl) || rl.RetryAfter.Seconds() != 42 {
		t.Fatalf("err = %v", err)
	}
}
