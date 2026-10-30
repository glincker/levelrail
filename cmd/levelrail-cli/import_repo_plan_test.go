package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const repoPlanJSON = `{"repo_url":"https://github.com/acme/mono","branch":"main","package_cap":60,"packages_found":2,
"packages":[
 {"name":"mono-web","package_name":"web","directory":"apps/web","port":3000,"build_kind":"buildpack","depends_on":["packages/ui"],"watch_paths":[],
  "warnings":[{"code":"no_dockerfile","message":"no Dockerfile in this package"},{"code":"duplicate_port","message":"port 3000 is also proposed"}]},
 {"name":"mono-api","package_name":"api","directory":"apps/api","port":null,"build_kind":"dockerfile","depends_on":[],"watch_paths":[],"warnings":[]}],
"warnings":[],"totals":{"packages":2,"warnings":2,"flagged":1}}`

func TestRun_ImportRepoPlan_PrintsTable(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(repoPlanJSON))
	}))
	defer srv.Close()

	stdout, _ := runCLIExpectOK(t, []string{"import", "repo", "plan", "https://github.com/acme/mono", "--branch", "dev", "--dir", "apps", "--api-url", srv.URL})
	if gotPath != "/api/v1/imports/repo/plan" || !strings.Contains(gotBody, `"branch":"dev"`) || !strings.Contains(gotBody, `"base_directory":"apps"`) {
		t.Errorf("path = %s body = %s", gotPath, gotBody)
	}
	for _, want := range []string{"NAME", "mono-web", "apps/web", "3000", "buildpack", "packages/ui", "no_dockerfile,duplicate_port", "mono-api", "2 packages, 2 warnings", "warning: mono-web: no Dockerfile"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRun_ImportRepoPlan_JSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(repoPlanJSON))
	}))
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"import", "repo", "plan", "https://github.com/acme/mono", "--json", "--api-url", srv.URL})
	if !strings.Contains(stdout, `"package_name": "web"`) && !strings.Contains(stdout, `"package_name":"web"`) {
		t.Errorf("json output = %s", stdout)
	}
}
