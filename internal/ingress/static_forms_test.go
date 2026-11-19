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
	if !strings.Contains(s, `"/_forms/*"`) || !strings.Contains(s, "/api/v1/forms/site/{http.request.uri.path.1}") {
		t.Fatalf("forms route missing: %s", s)
	}
	if strings.Index(s, "/_forms/*") > strings.Index(s, "file_server") && strings.Contains(s, "file_server") {
		t.Fatal("forms route must precede the file server")
	}
}
