package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const previewStatusJSON = `{"app":"web","enabled":true,"path":"/pricing","wait_ms":0,"server_enabled":true,"capturing":false,"image":"browser:1","keep_per_app":5,"ttl_days":30,"max_total_mb":200,"storage":{"app_bytes":2048,"app_count":2,"total_bytes":4096,"total_count":3},"latest":{"deployment_id":"dep_1","status":"skipped","reason":"auth_wall","detail":"the app answered 401","path":"/pricing","bytes":0,"captured_at":"2026-09-01T10:00:00Z"}}`

func TestRun_Preview_Verbs(t *testing.T) {
	type call struct{ method, path, body string }
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		calls = append(calls, call{r.Method, r.URL.Path, strings.TrimSpace(string(b))})
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/prune") {
			_, _ = w.Write([]byte(`{"removed":3,"freed_bytes":3072}`))
			return
		}
		_, _ = w.Write([]byte(previewStatusJSON))
	}))
	defer srv.Close()
	api := []string{"--api-url", srv.URL}

	out, _ := runCLIExpectOK(t, append([]string{"preview", "status", "web"}, api...))
	for _, want := range []string{"previews:   on (path /pricing", "2 previews", "last 5 per app", "auth_wall"} {
		if !strings.Contains(out, want) {
			t.Errorf("status output missing %q: %s", want, out)
		}
	}

	runCLIExpectOK(t, append([]string{"preview", "enable", "web", "--path", "/pricing", "--wait-ms", "300"}, api...))
	runCLIExpectOK(t, append([]string{"preview", "enable", "web", "--mode", "metadata"}, api...))
	runCLIExpectOK(t, append([]string{"preview", "disable", "web", "--mode", "screenshot"}, api...))
	runCLIExpectOK(t, append([]string{"preview", "disable", "web"}, api...))
	runCLIExpectOK(t, append([]string{"preview", "capture", "web"}, api...))
	out, _ = runCLIExpectOK(t, append([]string{"preview", "prune", "web", "--all"}, api...))
	if !strings.Contains(out, "removed 3 previews") {
		t.Errorf("prune output = %s", out)
	}

	want := []call{
		{http.MethodGet, "/api/v1/apps/web/preview", ""},
		{http.MethodPut, "/api/v1/apps/web/preview", `{"enabled":true,"path":"/pricing","wait_ms":300}`},
		{http.MethodPut, "/api/v1/apps/web/preview", `{"mode":"metadata","enabled":true}`},
		{http.MethodPut, "/api/v1/apps/web/preview", `{"enabled":false}`},
		{http.MethodPut, "/api/v1/apps/web/preview", `{"enabled":false}`},
		{http.MethodPost, "/api/v1/apps/web/preview/capture", ""},
		{http.MethodPost, "/api/v1/apps/web/preview/prune", `{"all":true}`},
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %+v, want %d", calls, len(want))
	}
	for i, w := range want {
		if calls[i] != w {
			t.Errorf("call %d = %+v, want %+v", i, calls[i], w)
		}
	}
}

func TestRun_Preview_UsageErrors(t *testing.T) {
	for _, args := range [][]string{{"preview"}, {"preview", "bogus"}, {"preview", "status"}} {
		var o, e strings.Builder
		if code := run("cli", args, &o, &e, envMap()); code != exitUsage {
			t.Errorf("%v exit = %d, want usage", args, code)
		}
	}
}
