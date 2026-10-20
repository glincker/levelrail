package apiclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWithNoCredentialsFix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"authentication required"}`))
	}))
	defer srv.Close()

	tests := []struct {
		name    string
		token   string
		wantMsg string
	}{
		{"no token gets the fix", "", "run the fix"},
		{"a rejected token keeps the server message", "stale", "authentication required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClient(srv.URL, tt.token, WithNoCredentialsFix("run the fix"))
			err := c.do(context.Background(), http.MethodGet, "/x", nil, nil)
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.Message != tt.wantMsg {
				t.Fatalf("err = %v, want APIError message %q", err, tt.wantMsg)
			}
		})
	}
}
