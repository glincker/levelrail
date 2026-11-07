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
			NextSkew: &NextSkewRoute{HeldDial: "127.0.0.1:9001", DeploymentIDs: []string{"abc-1", "bad id'"}},
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
	if _, ok := (&NextSkewRoute{HeldDial: "x:1", DeploymentIDs: []string{"a b"}}).route([]string{"h"}, nil); ok {
		t.Fatal("no safe id must add no route")
	}
	if _, ok := (*NextSkewRoute)(nil).route([]string{"h"}, nil); ok {
		t.Fatal("nil must add no route")
	}
}
