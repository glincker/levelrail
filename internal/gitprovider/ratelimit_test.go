package gitprovider

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsRateLimited(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		header    map[string]string
		body      string
		wantLimit bool
		wantAfter string
	}{
		{"429 with retry-after", 429, map[string]string{"Retry-After": "30"}, "slow", true, "30"},
		{"403 exhausted quota", 403, map[string]string{"X-RateLimit-Remaining": "0"}, "no", true, ""},
		{"403 rate limit body", 403, nil, "You have exceeded a secondary Rate Limit", true, ""},
		{"403 permission", 403, nil, "Resource not accessible by integration", false, ""},
		{"500", 500, nil, "boom", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()
			req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
			err := Execute(srv.Client(), req, "p", "api", "GET /", nil)
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want an *APIError", err)
			}
			after, limited := IsRateLimited(fmt.Errorf("wrapped: %w", err))
			if limited != tt.wantLimit || after != tt.wantAfter {
				t.Fatalf("IsRateLimited = (%q, %v), want (%q, %v)", after, limited, tt.wantAfter, tt.wantLimit)
			}
		})
	}
	if _, limited := IsRateLimited(errors.New("plain")); limited {
		t.Fatal("a plain error is not rate limited")
	}
}
