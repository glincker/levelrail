package application

import (
	"context"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

// A one-replica restart must leave the new container plus at most the one
// held previous release, however many resync passes run and however many
// restarts land inside the hold window.
func TestRestart_OneReplicaNeverExceedsTwoContainers(t *testing.T) {
	srv := alwaysHealthy()
	defer srv.Close()
	rt := newFakeRuntime(serverPort(t, srv))
	cfg := deploysafety.Effective{SuccessThreshold: 1, DrainTimeout: 15 * time.Second}
	p := clockedPass{t: t, rt: rt, tracker: NewDrainTracker(), cfg: cfg, hold: 3 * time.Minute}
	desired := *gateDesired()
	desired.Image = "img:v1"
	nonces := []string{"", "n1", "n2", "n3"}
	for _, n := range nonces {
		rt.setExitState(ContainerName("web", "img:v1", n), &docker.ExitState{Running: true})
	}
	t0 := time.Now().Truncate(time.Second).Add(-time.Hour)
	at := t0
	for i, n := range nonces {
		desired.RestartNonce = n
		for pass := 0; pass < 6; pass++ {
			p.run(at, desired)
			at = at.Add(10 * time.Second)
			if got := len(rt.names()); got > 2 {
				t.Fatalf("restart %d pass %d: %d containers %v, want at most 2", i, pass, got, rt.names())
			}
		}
	}
	p.run(at.Add(10*time.Minute), desired)
	if got := rt.names(); len(got) != 1 {
		t.Fatalf("after the hold: %v, want exactly the current container", got)
	}
}

// A restart changes the target name, so a duplicate could only come from the
// same name being created twice. Each restart must create its target once,
// however many resync passes follow. The engine serializes passes per service,
// so overlapping passes on one controller are not a supported shape.
func TestRestart_RepeatedPassesCreateEachTargetOnce(t *testing.T) {
	rt := newFakeRuntime(0)
	desired := &store.DesiredService{Name: "web", Image: "img:v1", Port: 80}
	c := New("web", &fakeStore{svc: desired}, rt)
	for i, nonce := range []string{"", "n1", "n2"} {
		desired.RestartNonce = nonce
		for pass := 0; pass < 4; pass++ {
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatalf("restart %d pass %d: %v", i, pass, err)
			}
		}
		want := "create:" + ContainerName("web", "img:v1", nonce)
		n := 0
		rt.mu.Lock()
		for _, call := range rt.callOrder {
			if call == want {
				n++
			}
		}
		rt.mu.Unlock()
		if n != 1 {
			t.Fatalf("restart %d: %q called %d times, want 1", i, want, n)
		}
	}
}
