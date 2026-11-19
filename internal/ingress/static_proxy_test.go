package ingress

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func proxyConfig(t *testing.T, r StaticRoute) string {
	t.Helper()
	r.Hosts, r.RootDir = []string{"a.example.com"}, t.TempDir()
	cfg, err := BuildRoutesConfig(RoutesOptions{ServerName: "ingress", ListenAddr: ":8443", StaticRoutes: []StaticRoute{r}})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cfg)
	return string(raw)
}

func TestStaticRouteProxyRoute(t *testing.T) {
	s := proxyConfig(t, StaticRoute{
		Proxies:   []StaticProxy{{From: "/api/*", Path: "/v1/:splat", Dial: "10.0.0.5:3000"}},
		BasicAuth: &BasicAuthAccount{Username: "site", Password: "hash"},
	})
	for _, want := range []string{"10.0.0.5:3000", "/v1/{http.regexp.lrproxy0.1}?{http.request.uri.query}", `"path_regexp"`, "X-Forwarded-Host", `"handler":"authentication"`} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in %s", want, s)
		}
	}
	if strings.Index(s, "10.0.0.5:3000") > strings.Index(s, "levelrail_static_site") && strings.Contains(s, "levelrail_static_site") {
		t.Fatal("proxy route must precede the static handler")
	}
	if strings.Index(s, `"authentication"`) > strings.Index(s, "10.0.0.5:3000") {
		t.Fatal("the site gate must run before the proxy")
	}
}

func TestStaticRouteProxyUnavailableAnswers502(t *testing.T) {
	s := proxyConfig(t, StaticRoute{Proxies: []StaticProxy{{From: "/api/*", Path: "/:splat"}}})
	if !strings.Contains(s, `"status_code":502`) || strings.Contains(s, "reverse_proxy") {
		t.Fatalf("expected a fixed 502 and no proxy: %s", s)
	}
}

func TestProxyMatchPatterns(t *testing.T) {
	pat, splat := proxyMatchPattern("/api/*")
	re := regexp.MustCompile(pat)
	if !splat || re.FindStringSubmatch("/api/users/1")[1] != "users/1" || !re.MatchString("/api") || re.MatchString("/apix/y") {
		t.Fatalf("splat pattern %s", pat)
	}
	for _, bad := range []string{"/api/a b", "/api/a?b", "/api/a#b", "/api/é"} {
		if re.MatchString(bad) {
			t.Fatalf("%q must not match", bad)
		}
	}
	exact, _ := proxyMatchPattern("/health")
	if !regexp.MustCompile(exact).MatchString("/health/") || regexp.MustCompile(exact).MatchString("/health/x") {
		t.Fatalf("exact pattern %s", exact)
	}
	guard := regexp.MustCompile(proxyDotSegmentPattern("/api/*"))
	for _, p := range []string{"/api/../admin", "/api/a/./b", "/api/%2e%2E/x", "/api/.."} {
		if !guard.MatchString(p) {
			t.Fatalf("guard must catch %q", p)
		}
	}
	if guard.MatchString("/api/a.b/c") {
		t.Fatal("guard must not catch dotted names")
	}
}

func TestProxySplatRejectsPathTricks(t *testing.T) {
	pat, _ := proxyMatchPattern("/api/*")
	re := regexp.MustCompile(pat)
	for _, bad := range []string{"/api/..;/admin", "/api/..%252fadmin", "/api/%2e%2e/x", "/api/a;b", "/api/a%2fb"} {
		if re.MatchString(bad) {
			t.Fatalf("%q must not match the proxy", bad)
		}
	}
}

func TestProxyConfigEscapesBraces(t *testing.T) {
	s := proxyConfig(t, StaticRoute{Proxies: []StaticProxy{{From: "/x", Path: "/{env.FOO}/{file./etc/hostname}", Dial: "10.0.0.5:3000"}}})
	if strings.Contains(s, "{env.FOO}") && !strings.Contains(s, `\\{env.FOO\\}`) {
		t.Fatalf("unescaped placeholder in %s", s)
	}
	if strings.Contains(strings.ReplaceAll(s, `\\{`, ""), "{env.") || strings.Contains(strings.ReplaceAll(s, `\\{`, ""), "{file.") {
		t.Fatalf("unescaped placeholder in %s", s)
	}
}
