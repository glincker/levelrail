package ingress

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveWith(t *testing.T, info HostInfo, show bool, accept string) (int, string) {
	t.Helper()
	h := DiagnosticHandler(func(context.Context, string) HostInfo { return info },
		func(context.Context) bool { return show }, "Levelrail", "https://d.example")
	req := httptest.NewRequest(http.MethodGet, "http://shop.example:8443/", nil)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code, rec.Body.String()
}

func TestDiagnosticStates(t *testing.T) {
	cases := []struct {
		info   HostInfo
		status int
		head   string
		cmd    string
	}{
		{HostInfo{StateUnattached}, 404, "not attached to an app", "levelrail-cli domains add &lt;app&gt; &lt;host&gt;"},
		{HostInfo{StateBuilding}, 503, "still building", "levelrail-cli apps deploys list &lt;app&gt;"},
		{HostInfo{StateStopped}, 503, "The app is stopped", "levelrail-cli apps start &lt;app&gt;"},
		{HostInfo{StateStarting}, 503, "The app is starting", "levelrail-cli logs &lt;app&gt;"},
		{HostInfo{StateUnhealthy}, 503, "no healthy replica", "levelrail-cli logs &lt;app&gt;"},
		{HostInfo{StateUnknown}, 503, "No app is serving this address", "levelrail-cli domains list"},
		{HostInfo{StateNotLoaded}, 404, "No app is serving this address", "levelrail-cli domains list"},
	}
	for _, c := range cases {
		code, body := serveWith(t, c.info, true, "text/html")
		if code != c.status || !strings.Contains(body, c.head) || !strings.Contains(body, c.cmd) {
			t.Errorf("%s: code %d, body missing %q / %q", c.info.State, code, c.head, c.cmd)
		}
		_, plain := serveWith(t, c.info, true, "")
		if strings.Contains(plain, "<") {
			t.Errorf("%s: plain text must be short: %q", c.info.State, plain)
		}
	}
}

// With the setting off every state gets the identical static 404 answer.
func TestDiagnosticHidesStateByDefault(t *testing.T) {
	wantCode, wantBody := serveWith(t, HostInfo{StateUnattached}, false, "text/html")
	if wantCode != 404 || strings.Contains(wantBody, "not attached to an app") {
		t.Fatalf("generic response wrong: %d", wantCode)
	}
	for _, st := range []HostState{StateBuilding, StateStopped, StateStarting, StateUnhealthy, StateUnknown, StateNotLoaded} {
		for _, accept := range []string{"text/html", ""} {
			code, body := serveWith(t, HostInfo{st}, false, accept)
			_, ref := serveWith(t, HostInfo{StateUnattached}, false, accept)
			if code != 404 || body != ref {
				t.Errorf("%s (%q): not the generic response: %d", st, accept, code)
			}
		}
	}
}

func TestNormalizeHostTable(t *testing.T) {
	cases := map[string]string{
		"Web.Example.COM":       "web.example.com",
		"web.example.com.":      "web.example.com",
		"web.example.com:8443":  "web.example.com",
		"[::1]:8080":            "::1",
		"bücher.example":        "xn--bcher-kva.example",
		"xn--bcher-kva.example": "xn--bcher-kva.example",
	}
	for in, want := range cases {
		if got := NormalizeHost(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}
