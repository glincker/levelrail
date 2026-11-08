package ingress

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile/application"
	"github.com/GLINCKER/levelrail/internal/store"
)

func skewContainer(name, image string, hostPort int, age time.Duration) docker.ContainerState {
	return docker.ContainerState{
		Name: name, Image: image, Running: true, Created: time.Now().Add(-age),
		Ports: []docker.PortBinding{{ContainerPort: 80, HostPort: hostPort}},
	}
}

func TestNextSkewForExcludesEveryCurrentReplicaAndMapsIDsToReleases(t *testing.T) {
	svc := store.DesiredService{Name: "web", Image: "img:v3", Port: 80, Replicas: 3}
	cur := func(i int) string { return application.ReplicaContainerName(svc.Name, svc.Image, svc.RestartNonce, i) }
	oldA := application.ReplicaContainerName(svc.Name, "img:v2", "", 0)
	oldB := application.ReplicaContainerName(svc.Name, "img:v1", "", 0)
	rt := &prefixRuntime{fakeRuntime: newFakeRuntime(), byPrefix: []docker.ContainerState{
		skewContainer(cur(0), "img:v3", 9100, 0),
		skewContainer(cur(1), "img:v3", 9101, 0),
		skewContainer(cur(2), "img:v3", 9102, 0),
		skewContainer(oldA, "img:v2", 9200, time.Hour),
		skewContainer(oldB, "img:v1", 9300, 2*time.Hour),
	}}
	c := New(&fakeStore{}, rt, &fakeApplier{}, WithLogger(discardLogger()), WithNextGraces(func(string, time.Time) []store.NextAssetGrace {
		return []store.NextAssetGrace{
			{Service: "web", DeploymentID: "id-v2", Image: "img:v2"},
			{Service: "web", DeploymentID: "id-v1", Image: "img:v1"},
		}
	}))

	got := c.nextSkewFor(context.Background(), svc, "127.0.0.1:9100", time.Now())
	if len(got) != 2 {
		t.Fatalf("want one route per held release, got %+v", got)
	}
	want := map[string]string{"id-v2": "127.0.0.1:9200", "id-v1": "127.0.0.1:9300"}
	for _, r := range got {
		for _, id := range r.DeploymentIDs {
			if want[id] != r.HeldDial {
				t.Errorf("id %s routed to %s, want %s", id, r.HeldDial, want[id])
			}
		}
		switch r.HeldDial {
		case "127.0.0.1:9100", "127.0.0.1:9101", "127.0.0.1:9102":
			t.Errorf("current release replica %s must never receive old ids", r.HeldDial)
		}
	}
}
