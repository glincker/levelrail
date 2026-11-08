package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsContainers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/apps/web/containers" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"0123456789ab","image":"img:v1","state":"running","node":"local","ports":[{"container":8080,"host":33001,"proto":"tcp","bind":"127.0.0.1"}],"networks":[],"mounts":[],"limits":{},"unavailable":["health"]}]`))
	}))
	t.Cleanup(srv.Close)

	stdout, _ := runCLIExpectOK(t, []string{"apps", "containers", "web", "--api-url", srv.URL})
	if !strings.Contains(stdout, "0123456789ab") || !strings.Contains(stdout, "127.0.0.1:33001->8080/tcp") {
		t.Errorf("stdout = %q", stdout)
	}
	jsonOut, _ := runCLIExpectOK(t, []string{"apps", "containers", "web", "--json", "--api-url", srv.URL})
	if !strings.Contains(jsonOut, `"unavailable"`) {
		t.Errorf("json stdout = %q", jsonOut)
	}
}
