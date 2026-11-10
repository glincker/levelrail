package apiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// An id with ?, # or .. must reach the server as one path segment, so the id a
// user confirmed is the id that is revoked or revealed.
func TestSignInIDsAreOnePathSegment(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.EscapedPath()+" q="+r.URL.RawQuery)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := NewClient(srv.URL, "test-token")
	ctx := context.Background()
	if err := c.RevokeTrustedDevice(ctx, "td?x=1#frag"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RevealLoginCode(ctx, "../trusted-devices"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"DELETE /api/v1/auth/trusted-devices/td%3Fx=1%23frag q=",
		"POST /api/v1/auth/sign-in-requests/codes/..%2Ftrusted-devices/reveal q=",
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("requests = %q, want %q", got, want)
	}
}
