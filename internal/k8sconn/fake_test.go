package k8sconn

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/trafficpolicy"
)

// newFake serves canned JSON by path and returns a client pinned to it.
func newFake(t *testing.T, routes map[string]string) *Client {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := routes[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(context.Background(), Config{
		Creds:   serverCreds(srv),
		Guard:   trafficpolicy.EgressGuard{AllowLoopback: true},
		Timeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return c
}
