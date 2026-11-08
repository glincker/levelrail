package main

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// oldServer answers like a release that predates these endpoints: the router has no such route.
func oldServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.NotFound(w, nil)
	}))
}

func TestCLIAgainstOlderServerReportsNotSupported(t *testing.T) {
	srv := oldServer(t)
	defer srv.Close()
	cases := map[string][]string{
		"next advisor":      {"apps", "next-advisor", "web"},
		"disk history":      {"system", "disk", "history"},
		"maintenance show":  {"apps", "maintenance", "show", "web"},
		"maintenance clear": {"apps", "maintenance", "clear", "web"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := run("levelrail-cli-test", append(args, "--api-url", srv.URL, "--token", "t"), &stdout, &stderr, envMap())
			if got != exitAPIError {
				t.Fatalf("exit = %d, want %d (stderr=%q)", got, exitAPIError, stderr.String())
			}
			if !strings.Contains(stderr.String(), "does not support") {
				t.Fatalf("stderr lacks the not supported hint: %q", stderr.String())
			}
		})
	}
}

func TestNotSupportedClassification(t *testing.T) {
	notFoundResource := &apiclient.APIError{StatusCode: 404, Message: `app "web" not found`}
	if apiclient.IsNotSupported(apiclient.NotSupported("x", notFoundResource)) {
		t.Fatal("a missing resource must stay a plain 404")
	}
	for _, e := range []*apiclient.APIError{
		{StatusCode: 404, Message: "404 page not found"},
		{StatusCode: 405, Message: "method not allowed"},
		{StatusCode: 501, Message: "nope"},
	} {
		got := apiclient.NotSupported("x", e)
		if !apiclient.IsNotSupported(got) {
			t.Fatalf("%d should be not supported", e.StatusCode)
		}
		var api *apiclient.APIError
		if !errors.As(got, &api) {
			t.Fatal("the APIError must stay reachable for exit codes")
		}
	}
	plain := errors.New("connection refused")
	if apiclient.NotSupported("x", plain) != plain {
		t.Fatal("network errors pass through")
	}
}

// An older server omits fields a newer CLI knows; decoding must not fail on them.
func TestCLIDecodesOlderServerPayload(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	got := run("levelrail-cli-test", []string{"system", "disk", "history", "--api-url", srv.URL, "--token", "t", "--json"}, &stdout, &stderr, envMap())
	if got != exitOK {
		t.Fatalf("exit = %d, stderr=%q", got, stderr.String())
	}
}
