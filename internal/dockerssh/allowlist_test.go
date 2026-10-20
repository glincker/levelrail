package dockerssh

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type okTransport struct{ body string }

func (o okTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: http.NoBody, Header: http.Header{}}, nil
}

func newReq(t *testing.T, method, target string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(method, target, nil)
	return r
}

func TestAllowlistAllowsOnlyReadOnlyGETs(t *testing.T) {
	tr := NewReadOnlyTransport(okTransport{})
	tr.learn("abc", []byte(`{"Id":"abc123","Name":"/web","Mounts":[{"Destination":"/data"}]}`))
	allowed := []string{
		"/_ping", "/version", "/info", "/v1.45/info",
		"/containers/json?all=1", "/containers/abc/json", "/v1.45/containers/abc/json",
		"/volumes", "/volumes/pgdata", "/networks", "/networks/bridge", "/images/json",
		"/images/nginx:latest/json", "/images/library/nginx:1.27/get", "/images/web@sha256:abcd/get",
		"/services", "/tasks", "/nodes",
		"/containers/abc/archive?path=/data", "/containers/abc123/archive?path=/data", "/containers/web/archive?path=/data",
	}
	for _, p := range allowed {
		if err := tr.Check(newReq(t, http.MethodGet, p)); err != nil {
			t.Errorf("GET %s refused: %v", p, err)
		}
	}
}

func TestAllowlistRefusesEverythingElse(t *testing.T) {
	tr := NewReadOnlyTransport(okTransport{})
	tr.learn("abc", []byte(`{"Id":"abc123","Name":"/web","Mounts":[{"Destination":"/data"}]}`))
	cases := []struct{ method, target string }{
		{"POST", "/containers/create"},
		{"POST", "/containers/abc/exec"},
		{"POST", "/exec/xyz/start"},
		{"POST", "/containers/abc/start"},
		{"POST", "/containers/abc/stop"},
		{"POST", "/containers/abc/attach"},
		{"POST", "/containers/abc/kill"},
		{"POST", "/commit"},
		{"POST", "/build"},
		{"POST", "/auth"},
		{"POST", "/session"},
		{"POST", "/images/create?fromImage=x"},
		{"POST", "/images/load"},
		{"POST", "/volumes/create"},
		{"POST", "/networks/create"},
		{"POST", "/swarm/init"},
		{"PUT", "/containers/abc/archive?path=/data"},
		{"DELETE", "/containers/abc"},
		{"DELETE", "/volumes/pgdata"},
		{"DELETE", "/images/nginx"},
		{"HEAD", "/containers/abc/archive?path=/data"},
		{"HEAD", "/_ping"},
		{"PATCH", "/info"},
		{"GET", "/containers/abc/logs?stdout=1"},
		{"GET", "/containers/abc/attach/ws"},
		{"GET", "/containers/abc/export"},
		{"GET", "/containers/abc/changes"},
		{"GET", "/containers/abc/stats"},
		{"GET", "/events"},
		{"GET", "/exec/xyz/json"},
		{"GET", "/secrets"},
		{"GET", "/configs"},
		{"GET", "/plugins"},
		{"GET", "/images/get?names=a"},
		{"GET", "/images/search?term=a"},
		{"GET", "/images/nginx/history"},
		{"GET", "/system/df"},
		{"GET", "/containers/abc/archive"},
		{"GET", "/containers/abc/archive?path=/etc"},
		{"GET", "/containers/abc/archive?path=/data/sub"},
		{"GET", "/containers/abc/archive?path=/data/../etc"},
		{"GET", "/containers/abc/archive?path=data"},
		{"GET", "/containers/other/archive?path=/data"},
		{"GET", "/containers/../etc/json"},
		{"GET", "/containers/abc/json/../../exec"},
		{"GET", "//containers/json"},
		{"GET", "/containers%2Fjson"},
		{"GET", "/images/../containers/x/get"},
		{"GET", "/v1.45/containers/abc/logs"},
		{"GET", "/"},
	}
	for _, c := range cases {
		err := tr.Check(newReq(t, c.method, c.target))
		if err == nil {
			t.Errorf("%s %s was allowed", c.method, c.target)
			continue
		}
		if !errors.Is(err, ErrRequestForbidden) {
			t.Errorf("%s %s: error %v does not wrap ErrRequestForbidden", c.method, c.target, err)
		}
		var fe *ForbiddenRequestError
		if !errors.As(err, &fe) || fe.Reason == "" {
			t.Errorf("%s %s: not a typed ForbiddenRequestError with a reason", c.method, c.target)
		}
	}
}

func TestAllowlistRefusesGETWithBody(t *testing.T) {
	tr := NewReadOnlyTransport(okTransport{})
	r := httptest.NewRequest(http.MethodGet, "/info", strings.NewReader("x"))
	if err := tr.Check(r); err == nil {
		t.Fatal("GET with a body was allowed")
	}
}

func TestRoundTripNeverReachesInnerWhenRefused(t *testing.T) {
	called := false
	inner := roundTripFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("must not be called")
	})
	tr := NewReadOnlyTransport(inner)
	for _, m := range []string{"POST", "PUT", "DELETE"} {
		if _, err := tr.RoundTrip(newReq(t, m, "/containers/create")); !errors.Is(err, ErrRequestForbidden) {
			t.Fatalf("%s: %v", m, err)
		}
	}
	if called {
		t.Fatal("a refused request reached the wire")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
