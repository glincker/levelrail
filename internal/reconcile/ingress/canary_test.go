package ingress

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeCanarySource struct{ releases []store.CanaryRelease }

func (f fakeCanarySource) ListCanaryReleases(context.Context) ([]store.CanaryRelease, error) {
	return f.releases, nil
}

func canaryFixture(weight int, seedCanary bool) (*fakeApplier, *Controller) {
	stable := lbService(1)
	canary := store.DesiredService{Name: store.CanaryServiceName("web"), Image: "img:v2", Port: 80, Replicas: 1}
	rt := newFakeRuntime()
	rt.seedRunning(replicaName(stable, 0), 30000)
	if seedCanary {
		rt.seedRunning(replicaName(canary, 0), 30100)
	}
	applier := &fakeApplier{}
	src := fakeCanarySource{releases: []store.CanaryRelease{{ServiceName: "web", CanaryService: canary.Name, Image: "img:v2", Weight: weight}}}
	c := New(&fakeStore{services: []store.DesiredService{stable, canary}}, rt, applier, WithLogger(discardLogger()), WithCanaries(src))
	return applier, c
}

func TestController_Canary_SplitsTrafficByWeight(t *testing.T) {
	applier, c := canaryFixture(20, true)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	rp := proxyHandler(t, applier)
	if len(rp.Upstreams) != 2 || rp.Upstreams[0].Dial != "127.0.0.1:30000" || rp.Upstreams[1].Dial != "127.0.0.1:30100" {
		t.Fatalf("upstreams = %+v", rp.Upstreams)
	}
	w := rp.LoadBalancing.SelectionPolicy.Weights
	if rp.LoadBalancing.SelectionPolicy.Policy != "weighted_round_robin" || len(w) != 2 || w[0] != 80 || w[1] != 20 {
		t.Errorf("policy = %+v", rp.LoadBalancing.SelectionPolicy)
	}
}

func TestController_Canary_NotReadyOrPausedRoutesStableOnly(t *testing.T) {
	for name, tc := range map[string]struct {
		weight int
		seed   bool
	}{"canary not running": {20, false}, "paused at zero": {0, true}} {
		t.Run(name, func(t *testing.T) {
			applier, c := canaryFixture(tc.weight, tc.seed)
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			rp := proxyHandler(t, applier)
			if len(rp.Upstreams) != 1 || rp.Upstreams[0].Dial != "127.0.0.1:30000" {
				t.Errorf("upstreams = %+v, want stable only", rp.Upstreams)
			}
		})
	}
}
