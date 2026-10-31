package application

import (
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/deploysafety"
	"github.com/GLINCKER/levelrail/internal/docker"
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
