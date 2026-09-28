package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_LB_List(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"app":"web","service":"web","algorithm":"least_conn","state":"degraded","upstreams_total":2,"upstreams_healthy":1}],"total":3,"limit":100,"offset":0}`))
	}))
	defer srv.Close()

	out, _ := runCLIExpectOK(t, []string{"lb", "list", "--state", "degraded", "--search", "we", "--api-url", srv.URL})
	for _, want := range []string{"APP", "web", "least_conn", "degraded", "1/2 healthy", "showing 1 of 3"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q: %s", want, out)
		}
	}
	if gotQuery != "q=we&state=degraded" {
		t.Errorf("query = %q", gotQuery)
	}

	out, _ = runCLIExpectOK(t, []string{"lb", "list", "--json", "--api-url", srv.URL})
	if !strings.Contains(out, `"upstreams_healthy": 1`) && !strings.Contains(out, `"upstreams_healthy":1`) {
		t.Errorf("json output = %s", out)
	}

	var o, e strings.Builder
	if code := run("cli", []string{"lb", "list", "extra"}, &o, &e, envMap()); code != exitUsage {
		t.Errorf("extra arg exit = %d, want usage", code)
	}
}
