package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsSleep(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		var b bytes.Buffer
		_, _ = b.ReadFrom(r.Body)
		gotBody = b.String()
		_ = json.NewEncoder(w).Encode(appSleepResource{Enabled: true, IdleMinutes: 45})
	}))
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		args               []string
		method, path, body string
	}{
		{[]string{"enable", "web", "--after", "45m"}, http.MethodPut, "/api/v1/apps/web/sleep", `{"idle_minutes":45}`},
		{[]string{"disable", "web"}, http.MethodPut, "/api/v1/apps/web/sleep", `{"idle_minutes":0}`},
		{[]string{"status", "web"}, http.MethodGet, "/api/v1/apps/web/sleep", ""},
		{[]string{"wake", "web"}, http.MethodPost, "/api/v1/apps/web/sleep/wake", ""},
	} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"apps", "sleep"}, tc.args...)
		args = append(args, "--token", "t", "--api-url", srv.URL)
		if got := run("levelrail-cli-test", args, &stdout, &stderr, envMap()); got != exitOK {
			t.Fatalf("%v: exit = %d (stderr=%q)", tc.args, got, stderr.String())
		}
		if gotMethod != tc.method || gotPath != tc.path || strings.TrimSpace(gotBody) != tc.body {
			t.Errorf("%v: %s %s %q, want %s %s %q", tc.args, gotMethod, gotPath, gotBody, tc.method, tc.path, tc.body)
		}
	}
}
