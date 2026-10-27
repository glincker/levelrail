package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

// A port another app took after the build was held refuses the publish and
// keeps the build held, so it can be published once the port is free.
func TestPublishLock_PublishHeldBuildRechecksPorts(t *testing.T) {
	stubUDPListeners(t)
	rt, db, cookie := newLockRouter(t)
	ctx := context.Background()
	udp := []store.PublishedPort{{ContainerPort: 53, HostPort: 5353, Protocol: spec.PortProtocolUDP}}
	if err := db.SaveDeployAttempt(ctx, store.DeployAttempt{ID: "dep_ports", ServiceName: "web", Image: "web:new", Status: store.DeployAttemptStatusSucceeded, StartedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveHeldBuild(ctx, "dep_ports", store.DesiredService{Name: "web", Image: "web:new", Port: 80, PublishedPorts: udp}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: "dns", Image: "dns:1", PublishedPorts: udp}); err != nil {
		t.Fatal(err)
	}
	if got := lockCall(t, rt, cookie, http.MethodPost, "/deploys/dep_ports/publish", ""); got != http.StatusConflict {
		t.Fatalf("publish = %d, want 409", got)
	}
	if got := liveImage(t, db.DB); got != "web:live" {
		t.Errorf("live image = %q, want web:live", got)
	}
	if _, err := db.GetHeldBuild(ctx, "dep_ports"); err != nil {
		t.Errorf("held build gone after refusal: %v", err)
	}
}
