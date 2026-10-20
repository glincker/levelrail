package ingress

import "testing"

func TestRedirectLocation(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   string
	}{
		{"bare origin keeps the request uri", "https://example.com", "https://example.com{http.request.uri}"},
		{"trailing slash origin keeps the request uri", "https://example.com/", "https://example.com{http.request.uri}"},
		{"origin with port keeps the request uri", "https://example.com:8443", "https://example.com:8443{http.request.uri}"},
		{"target with a path stays fixed", "https://newapp.example.com/promo", "https://newapp.example.com/promo"},
		{"target with a query stays fixed", "https://example.com/?ref=old", "https://example.com/?ref=old"},
		{"target with a fragment stays fixed", "https://example.com#top", "https://example.com#top"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redirectLocation(tt.target); got != tt.want {
				t.Errorf("redirectLocation(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}
