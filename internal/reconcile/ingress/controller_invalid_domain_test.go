package ingress

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/glincker/levelrail/internal/reconcile/application"
	"github.com/glincker/levelrail/internal/store"
)

func TestController_Reconcile_InvalidStoredDomainNeverReachesCaddy(t *testing.T) {
	svc := store.DesiredService{
		Name: "web", Image: "img:v1", Port: 80,
		Domains: []string{"{http.request.host}", "a/b", "bücher.example", "ok.example.com"},
	}
	rt := newFakeRuntime()
	rt.seedRunning(application.ContainerName(svc.Name, svc.Image, ""), 11111)
	applier := &fakeApplier{}
	c := New(&fakeStore{services: []store.DesiredService{svc}}, rt, applier, WithLogger(slog.New(slog.DiscardHandler)))

	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	var hosts []string
	for _, r := range applier.routes(t) {
		hosts = append(hosts, r.Match[0].Host...)
	}
	got := strings.Join(hosts, ",")
	if strings.ContainsAny(got, "{}/") || !strings.Contains(got, "xn--bcher-kva.example") || !strings.Contains(got, "ok.example.com") {
		t.Fatalf("routed hosts = %q", got)
	}
}
