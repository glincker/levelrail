package upgrade

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// The update check must stay anonymous: no Instance ID, cookie or custom header leaves the server.
func TestUpdateCheckRequestsCarryNoInstanceID(t *testing.T) {
	var mu sync.Mutex
	var seen []*http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Clone(context.Background()))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/commits/") {
			_, _ = w.Write([]byte(`{"sha":"07cd3bf9a1b2c3d4e5f60718293a4b5c6d7e8f90","html_url":"https://example.invalid/c","commit":{"committer":{"date":"2026-10-01T00:00:00Z"}}}`))
			return
		}
		_, _ = w.Write([]byte(`[{"tag_name":"v1.0.0","html_url":"https://example.invalid/r","assets":[]}]`))
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	old := channelHTTPClient
	t.Cleanup(func() { channelHTTPClient = old })
	channelHTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r2 := r.Clone(r.Context())
		r2.URL.Scheme, r2.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(r2)
	})}

	f := DefaultFetchers("Acme/Rail")
	for _, fn := range []func(context.Context) (*Release, error){f.Stable, f.Beta, f.Edge} {
		_, _ = fn(context.Background())
	}

	mu.Lock()
	defer mu.Unlock()
	if len(seen) == 0 {
		t.Fatal("no request reached the fake server")
	}
	allowed := map[string]bool{"Accept": true, "User-Agent": true, "Accept-Encoding": true}
	for _, r := range seen {
		if r.URL.Query().Has("instance_id") || strings.Contains(strings.ToLower(r.URL.RawQuery), "inst") {
			t.Errorf("%s carries identity in the query: %q", r.URL.Path, r.URL.RawQuery)
		}
		for name, vals := range r.Header {
			if !allowed[name] {
				t.Errorf("%s sends unexpected header %s", r.URL.Path, name)
			}
			for _, v := range vals {
				if strings.Contains(strings.ToLower(v), "inst_") {
					t.Errorf("%s header %s carries an instance id", r.URL.Path, name)
				}
			}
		}
		if r.ContentLength > 0 {
			t.Errorf("%s sends a body", r.URL.Path)
		}
	}
}
