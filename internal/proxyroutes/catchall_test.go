package proxyroutes

import (
	"strings"
	"testing"
)

const goldenUnrouted = "# managed by levelrail\n" +
	"# Do not edit: rewritten on every reconcile. Turn off the unrouted setting to remove it.\n" +
	"http:\n  routers:\n" +
	"    levelrail-unrouted:\n      rule: PathPrefix(`/`)\n      entryPoints: [websecure]\n      service: levelrail-unrouted\n      priority: -500\n      tls: {}\n" +
	"    levelrail-unrouted-http:\n      rule: PathPrefix(`/`)\n      entryPoints: [web]\n      service: levelrail-unrouted\n      priority: -500\n" +
	"  services:\n    levelrail-unrouted:\n      loadBalancer:\n        passHostHeader: true\n        servers:\n          - url: http://172.17.0.1:8080\n"

func TestRenderUnroutedGolden(t *testing.T) {
	got, err := Namespace("levelrail").RenderUnrouted(Route{
		EntrypointHTTP: "web", EntrypointHTTPS: "websecure", Upstream: "172.17.0.1:8080", Priority: DefaultUnroutedPriority,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != goldenUnrouted {
		t.Errorf("unrouted file differs from golden:\n%s", got)
	}
}

func TestRenderUnroutedRejectsBadPriorityAndName(t *testing.T) {
	ns := Namespace("levelrail")
	base := Route{EntrypointHTTP: "web", EntrypointHTTPS: "websecure", Upstream: "127.0.0.1:8080", Priority: -500}
	for _, prio := range []int{0, 5, -1000} {
		r := base
		r.Priority = prio
		if _, err := ns.RenderUnrouted(r); err == nil {
			t.Errorf("priority %d must be refused", prio)
		}
	}
	r := base
	r.EntrypointHTTP = "web\nx"
	if _, err := ns.RenderUnrouted(r); err == nil || !strings.Contains(err.Error(), "entrypoint") {
		t.Errorf("bad entrypoint must be refused: %v", err)
	}
}

func TestUnroutedFileName(t *testing.T) {
	if got := Namespace("levelrail").UnroutedFileName(".yaml"); got != "levelrail-managed-unrouted.yaml" {
		t.Errorf("got %q", got)
	}
}
