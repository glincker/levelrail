package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStaticRouteFormsRoute(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "ingress", ListenAddr: ":8443", StaticRoutes: []StaticRoute{{
		Hosts: []string{"a.example.com"}, RootDir: t.TempDir(), FormsDial: "127.0.0.1:8080", FormsApp: "site",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cfg)
	s := string(raw)
	if !strings.Contains(s, `"path_regexp"`) || !strings.Contains(s, "/api/v1/forms/site/{http.regexp.lrform.1}") || !strings.Contains(s, `"method":["POST","OPTIONS"]`) {
		t.Fatalf("forms route missing: %s", s)
	}
	if strings.Index(s, "path_regexp") > strings.Index(s, "file_server") && strings.Contains(s, "file_server") {
		t.Fatal("forms route must precede the file server")
	}
}

func TestFormsPathPatternRejectsTraversal(t *testing.T) {
	for _, p := range []string{"/_forms/..%252F..%252Fapps%252Fx", "/_forms/../x", "/_forms/a/b", "/_forms/a?x=1", "/_forms/", "/_forms/a%2Fb"} {
		if formsPathRe.MatchString(p) {
			t.Fatalf("%q must not match", p)
		}
	}
	if !formsPathRe.MatchString("/_forms/contact-1_x") {
		t.Fatal("plain name must match")
	}
}
