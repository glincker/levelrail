package ingress

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestNextSkewRouteBeforeHostRoute(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "s", ListenAddr: ":443",
		Routes: []ProxyRoute{{
			Hosts: []string{"app.example.com"}, BackendDial: "127.0.0.1:9000",
			NextSkew: []NextSkewRoute{{HeldDial: "127.0.0.1:9001", DeploymentIDs: []string{"abc-1", "bad id'"}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	out := string(raw)
	skew := strings.Index(out, "127.0.0.1:9001")
	current := strings.Index(out, "127.0.0.1:9000")
	if skew < 0 || current < 0 || skew > current {
		t.Fatalf("held route must precede the current route: %s", out)
	}
	for _, want := range []string{"/_next/static/*", "/_next/data/*", "{http.request.uri.query.dpl} == 'abc-1'", "{http.request.header.x-deployment-id} == 'abc-1'"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %s", want, out)
		}
	}
	if strings.Contains(out, "bad id") {
		t.Errorf("unsafe id leaked into the config: %s", out)
	}
}

func TestNextSkewRouteAbsentWithoutSafeIDs(t *testing.T) {
	if _, ok := (&NextSkewRoute{HeldDial: "x:1", DeploymentIDs: []string{"a b"}}).route([]string{"h"}, []any{NewReverseProxyHandler("a:1")}); ok {
		t.Fatal("no safe id must add no route")
	}
	if _, ok := (*NextSkewRoute)(nil).route([]string{"h"}, []any{NewReverseProxyHandler("a:1")}); ok {
		t.Fatal("nil must add no route")
	}
}

// findSkewRoute returns the marshaled route whose matcher is the skew expression.
func findSkewRoute(t *testing.T, cfg any) string {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatal(err)
	}
	var found string
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if m, ok := x["match"]; ok && strings.Contains(toJSON(t, m), "next_skew") {
				found = toJSON(t, x)
				return
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(tree)
	if found == "" {
		t.Fatalf("no skew route in %s", raw)
	}
	return found
}

func toJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestNextSkewRouteKeepsAuthChain(t *testing.T) {
	cfg, err := BuildRoutesConfig(RoutesOptions{
		ServerName: "s", ListenAddr: ":443",
		Routes: []ProxyRoute{{
			Hosts: []string{"gated.example.com"}, BackendDial: "127.0.0.1:9000",
			BasicAuth: &BasicAuthAccount{Username: "u", Password: "p"},
			NextSkew:  []NextSkewRoute{{HeldDial: "127.0.0.1:9001", DeploymentIDs: []string{"old-1"}}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	skew := findSkewRoute(t, cfg)
	for _, want := range []string{"authentication", "127.0.0.1:9001"} {
		if !strings.Contains(skew, want) {
			t.Errorf("skew route missing %q: %s", want, skew)
		}
	}
	if strings.Contains(skew, "127.0.0.1:9000") {
		t.Errorf("skew route must not dial the current release: %s", skew)
	}
	if strings.Index(skew, "authentication") > strings.Index(skew, "reverse_proxy") {
		t.Errorf("auth must run before the proxy: %s", skew)
	}
}
