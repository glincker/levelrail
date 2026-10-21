package migrationmode

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func mustURL(t testing.TB, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse("http://docker" + raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestAllowedPermitsOnlyReadCalls(t *testing.T) {
	for _, path := range []string{
		"/version", "/containers/json?all=1", "/containers/abc123/json", "/containers/my-app_1.web/json",
		"/volumes", "/volumes/data_vol", "/networks", "/v1.43/containers/json", "/v1.41/volumes/x",
	} {
		t.Run(path, func(t *testing.T) {
			if err := Allowed(http.MethodGet, mustURL(t, path)); err != nil {
				t.Fatalf("GET %s should be allowed: %v", path, err)
			}
		})
	}
}

func TestAllowedRejectsEveryForbiddenCall(t *testing.T) {
	cases := []struct{ method, path string }{
		{"POST", "/containers/abc/exec"},
		{"POST", "/exec/abc/start"},
		{"GET", "/exec/abc/json"},
		{"POST", "/containers/create"},
		{"POST", "/containers/abc/start"},
		{"POST", "/containers/abc/stop"},
		{"POST", "/containers/abc/kill"},
		{"POST", "/containers/abc/restart"},
		{"POST", "/containers/abc/pause"},
		{"POST", "/containers/abc/update"},
		{"POST", "/containers/abc/rename"},
		{"POST", "/containers/prune"},
		{"DELETE", "/containers/abc"},
		{"DELETE", "/containers/abc?force=1"},
		{"GET", "/containers/abc/archive"},
		{"PUT", "/containers/abc/archive"},
		{"HEAD", "/containers/abc/archive"},
		{"GET", "/containers/abc/export"},
		{"GET", "/containers/abc/logs"},
		{"GET", "/containers/abc/attach/ws"},
		{"POST", "/containers/abc/attach"},
		{"POST", "/containers/abc/wait"},
		{"GET", "/containers/abc/changes"},
		{"POST", "/images/create"},
		{"POST", "/images/load"},
		{"DELETE", "/images/abc"},
		{"POST", "/build"},
		{"POST", "/volumes/create"},
		{"DELETE", "/volumes/data"},
		{"POST", "/volumes/prune"},
		{"POST", "/networks/create"},
		{"DELETE", "/networks/abc"},
		{"POST", "/networks/abc/connect"},
		{"POST", "/system/prune"},
		{"GET", "/events"},
		{"GET", "/plugins"},
		{"GET", "/secrets"},
		{"GET", "/configs"},
		{"POST", "/swarm/init"},
		{"GET", "/swarm"},
		{"GET", "/nodes"},
		{"GET", "/services"},
		{"POST", "/auth"},
		{"POST", "/commit"},
		{"POST", "/session"},
		{"POST", "/version"},
		{"PUT", "/info"},
		{"GET", "/info"},
		{"GET", "/images/json"},
		{"GET", "/system/df"},
		{"GET", "/v1.43/info"},
		{"HEAD", "/containers/abc/json"},
		{"GET", "/containers/../exec/abc/json"},
		{"GET", "/containers/abc/../abc/exec"},
		{"GET", "/containers/%2e%2e/json"},
		{"GET", "/containers//json"},
		{"GET", "/containers/abc\\json"},
		{"GET", "/v1.43/containers/abc/archive"},
		{"POST", "/v1.43/containers/abc/exec"},
		{"GET", "/v1.43/v1.43/containers/json"},
		{"GET", "/containers/-abc/json"},
		{"GET", "/"},
		{"GET", ""},
		{"", "/version"},
		{"get", "/version"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			err := Allowed(tc.method, mustURL(t, tc.path))
			if !errors.Is(err, ErrDenied) {
				t.Fatalf("%s %s must be denied, got %v", tc.method, tc.path, err)
			}
		})
	}
}

type recordingTransport struct{ calls int }

func (r *recordingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	r.calls++
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func TestGuardBlocksBeforeTheSocketAndReportsDenials(t *testing.T) {
	next := &recordingTransport{}
	var denied []string
	rt := NewGuard(next, func(method, path, _ string) { denied = append(denied, method+" "+path) })

	req, _ := http.NewRequest(http.MethodPost, "http://docker/containers/x/exec", strings.NewReader("{}"))
	if _, err := rt.RoundTrip(req); !errors.Is(err, ErrDenied) {
		t.Fatalf("exec must be denied, got %v", err)
	}
	req, _ = http.NewRequest(http.MethodGet, "http://docker/version", strings.NewReader("x"))
	if _, err := rt.RoundTrip(req); !errors.Is(err, ErrDenied) {
		t.Fatalf("a GET with a body must be denied, got %v", err)
	}
	if next.calls != 0 {
		t.Fatalf("denied calls reached the socket %d times", next.calls)
	}
	if len(denied) != 2 {
		t.Fatalf("want 2 denial reports, got %v", denied)
	}
	req, _ = http.NewRequest(http.MethodGet, "http://docker/containers/json?all=1", http.NoBody)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if next.calls != 1 {
		t.Fatalf("allowed call did not pass through, calls=%d", next.calls)
	}
}

func FuzzAllowedNeverPermitsWrites(f *testing.F) {
	for _, s := range []string{"/containers/a/json", "/containers/a/exec", "/v1.43/volumes", "/../x", "/containers/%2e%2e/json"} {
		f.Add("POST", s)
		f.Add("DELETE", s)
		f.Add("PUT", s)
	}
	f.Fuzz(func(t *testing.T, method, path string) {
		u, err := url.Parse("http://docker" + path)
		if err != nil || method == http.MethodGet {
			return
		}
		if Allowed(method, u) == nil {
			t.Fatalf("%s %q was allowed", method, path)
		}
	})
}
