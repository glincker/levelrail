package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestCanaryRefusesPublishedPorts(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	ctx := context.Background()
	svc := store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, Replicas: 1,
		PublishedPorts: []store.PublishedPort{{ContainerPort: 53, HostPort: 5353, Protocol: "udp"}}}
	if err := db.SaveDesiredService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	if rec := canaryCall(t, rt.handleStartCanary, http.MethodPost, `{"image":"nginx:2"}`); rec.Code != http.StatusConflict {
		t.Fatalf("canary on a published port app = %d %s, want 409", rec.Code, rec.Body)
	}
	if _, err := db.GetDesiredService(ctx, store.CanaryServiceName("web")); err == nil {
		t.Error("a refused canary left a clone behind")
	}
}

func TestCloneDesiredServiceDropsHostPorts(t *testing.T) {
	hp := 8080
	src := store.DesiredService{Name: "web", Image: "nginx:1", Port: 80, HostPort: &hp,
		PublishedPorts: []store.PublishedPort{{ContainerPort: 53, HostPort: 53, Protocol: "udp"}}}
	got := cloneDesiredService(src, "web-copy", nil)
	if got.HostPort != nil || len(got.PublishedPorts) != 0 {
		t.Fatalf("clone kept host ports: host_port=%v published=%v", got.HostPort, got.PublishedPorts)
	}
}
