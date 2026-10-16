package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRun_AppsAnalyze_PrintsReasons(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"detection":{"framework":"Next.js","language":"node","build_type":"railpack","confidence":"high",
			"reasons":[{"signal":"package.json","detail":"package.json has next in dependencies"}],
			"port":{"value":3000,"source":"Next.js default port"},"health_path":"/",
			"env_vars":[{"name":"API_KEY","source":".env.example","required":true,"secret_guess":true}],
			"warnings":[{"code":"no_lockfile","message":"no lockfile"}]},"packages":[]}`))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"apps", "analyze", "https://github.com/acme/web", "--branch", "dev", "--dir", "apps/web", "--api-url", srv.URL})
	if gotPath != "/api/v1/git-sources/analyze" {
		t.Errorf("path = %s", gotPath)
	}
	if !strings.Contains(gotBody, `"directory":"apps/web"`) || !strings.Contains(gotBody, `"ref":"dev"`) {
		t.Errorf("body = %s", gotBody)
	}
	for _, want := range []string{"Next.js", "package.json has next in dependencies", "API_KEY required, secret", "warning: no lockfile"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}
