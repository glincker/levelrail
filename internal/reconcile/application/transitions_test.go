package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/bindaddr"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/spec"
	"github.com/GLINCKER/levelrail/internal/store"
)

const gamePort = 25565

func gameDesired(nonce string, hostPort *int, bind string) *store.DesiredService {
	return &store.DesiredService{
		Name: "game", Image: "game:1", Port: gamePort, HostPort: hostPort, BindAddress: bind,
		Replicas: 1, Strategy: "blue-green", RestartNonce: nonce,
		PublishedPorts: []store.PublishedPort{{ContainerPort: gamePort, HostPort: gamePort, Protocol: store.PortProtocolTCP, Bind: spec.PortBindPrivate}},
		Volumes:        []store.ServiceVolume{{Name: "game-data", ContainerPath: "/data"}},
	}
}

func reconcileN(t *testing.T, c *Controller, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := c.Reconcile(context.Background()); err != nil {
			t.Fatalf("Reconcile() pass %d error = %v", i, err)
		}
	}
}

func TestTransitions_StreamDeleteRestartStopsOldBeforeNew(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	st := &fakeStore{svc: gameDesired("", nil, bindaddr.Private), streams: []store.AppStream{{ID: "s1", ServiceName: "game", ContainerPort: gamePort, HostPort: gamePort, Protocol: "tcp"}}}
	c := New("game", st, rt)
	reconcileN(t, c, 1)

	// stream deleted and the app restarted: new name, same volume
	st.streams = nil
	st.svc = gameDesired("n1", nil, bindaddr.Private)
	rt.overlapStarts = 0
	reconcileN(t, c, 2)
	if rt.overlapStarts != 0 {
		t.Fatalf("a new container started while another held the volume (%d times)", rt.overlapStarts)
	}
	if got := runningNames(rt); len(got) != 1 {
		t.Fatalf("running = %v, want exactly one container on the volume", got)
	}
}

func TestTransitions_DirectToStreamHasNoDuplicateEphemeralBinding(t *testing.T) {
	hp := gamePort
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	st := &fakeStore{svc: gameDesired("", &hp, bindaddr.Public)}
	c := New("game", st, rt)
	reconcileN(t, c, 1)

	st.svc = gameDesired("n1", nil, bindaddr.Private)
	st.streams = []store.AppStream{{ID: "s1", ServiceName: "game", ContainerPort: gamePort, HostPort: gamePort, Protocol: "tcp"}}
	rt.overlapStarts = 0
	reconcileN(t, c, 2)

	ephemeral := 0
	for _, p := range rt.lastCreateSpec.Ports {
		if p.ContainerPort == gamePort && p.HostPort == 0 && p.HostIP == "127.0.0.1" {
			ephemeral++
		}
	}
	if ephemeral != 1 {
		t.Fatalf("ports = %+v, want one ephemeral loopback binding (Docker hands out the same host port twice otherwise)", rt.lastCreateSpec.Ports)
	}
	if rt.overlapStarts != 0 {
		t.Fatalf("overlapping starts = %d, want the direct release stopped first", rt.overlapStarts)
	}
	for _, n := range runningNames(rt) {
		for _, p := range rt.containers[n].Ports {
			if p.HostPort == gamePort {
				t.Fatalf("%s still holds host port %d that the stream listener needs", n, gamePort)
			}
		}
	}
}

func TestTransitions_StreamToDirectSwitch(t *testing.T) {
	hp := gamePort
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	st := &fakeStore{svc: gameDesired("", nil, bindaddr.Private), streams: []store.AppStream{{ID: "s1", ServiceName: "game", ContainerPort: gamePort, HostPort: gamePort, Protocol: "tcp"}}}
	c := New("game", st, rt)
	reconcileN(t, c, 1)

	st.streams = nil
	st.svc = gameDesired("n1", &hp, bindaddr.Public)
	rt.overlapStarts = 0
	reconcileN(t, c, 2)
	if got := runningNames(rt); len(got) != 1 {
		t.Fatalf("running = %v, want one container", got)
	}
	if rt.overlapStarts != 0 {
		t.Fatalf("overlapping starts = %d", rt.overlapStarts)
	}
}

func TestTransitions_SettledPassesCreateNothing(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	st := &fakeStore{svc: gameDesired("", nil, bindaddr.Private), streams: []store.AppStream{{ID: "s1", ServiceName: "game", ContainerPort: gamePort, HostPort: gamePort, Protocol: "tcp"}}}
	c := New("game", st, rt)
	reconcileN(t, c, 3)
	before := rt.createCalls
	ids := map[string]string{}
	for _, n := range rt.names() {
		ids[n] = rt.containers[n].ID
	}
	reconcileN(t, c, 3)
	if rt.createCalls != before {
		t.Fatalf("creates after settle = %d, want 0", rt.createCalls-before)
	}
	for n, id := range ids {
		if rt.containers[n] == nil || rt.containers[n].ID != id {
			t.Fatalf("container %s changed id across settled passes", n)
		}
	}
}

func TestTransitions_StatelessKeepsBlueGreenOverlap(t *testing.T) {
	v1 := replicaContainerName("web", "img:v1", "", 0)
	rt := newFakeRuntime(0)
	rt.seed(v1, true)
	rt.containers[v1].Ports = []docker.PortBinding{{ContainerPort: 80, HostPort: 40001, HostIP: "127.0.0.1"}}
	desired := &store.DesiredService{Name: "web", Image: "img:v2", Port: 80, Strategy: "blue-green", Replicas: 1}
	c := New("web", &fakeStore{svc: desired}, rt)
	reconcileN(t, c, 1)
	if rt.overlapStarts == 0 {
		t.Fatalf("a stateless app must keep start-first overlap")
	}
}

func TestTransitions_TCPOnlyAppReportsReadyWithoutHTTPRoute(t *testing.T) {
	rt := newFakeRuntime(0)
	rt.trackPorts = true
	st := &fakeStore{svc: gameDesired("", nil, bindaddr.Private)}
	c := New("game", st, rt)
	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("first pass condition = %+v, want Ready True", cond)
	}

	// stop then start: the very next pass writes a fresh True condition
	st.svc.Suspended = true
	reconcileN(t, c, 1)
	st.svc.Suspended = false
	res, err = c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile() after start error = %v", err)
	}
	if cond := conditionOf(t, res); cond.Status != reconcile.ConditionTrue {
		t.Fatalf("condition after start = %+v, want Ready True within one pass", cond)
	}
}
