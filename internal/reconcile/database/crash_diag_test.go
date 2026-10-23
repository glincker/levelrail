package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/store"
)

type crashRuntime struct {
	*fakeRuntime
	restarts int
	tail     string
}

func (r *crashRuntime) InspectExitState(context.Context, string) (*docker.ExitState, error) {
	return &docker.ExitState{ExitCode: 1, RestartCount: r.restarts}, nil
}

func (r *crashRuntime) TailLogs(context.Context, string, int) (string, error) { return r.tail, nil }

func TestProvision_CrashLoopStopsWithClassifiedReason(t *testing.T) {
	t.Setenv(EnvCrashLoopRestarts, "3")
	rt := &crashRuntime{fakeRuntime: newFakeRuntime(), restarts: 3, tail: "setpriv: apply bounding set: Operation not permitted"}
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	rt.containers[containerName("main")] = &docker.ContainerState{
		ID: "c1", Name: containerName("main"), Image: ImageRef(store.EngineRedis, "7"), Running: true,
		Created:  time.Now(),
		Networks: []docker.ContainerNetwork{{Name: "n", IPAddress: "10.0.0.5"}},
	}
	c, ms := gateFixture(t, rt, desired, WithReadyProbe(func(context.Context, string) error { return errors.New("refused") }))

	res, _ := c.Reconcile(context.Background())
	if res.Conditions[0].Reason != dbprovision.ReasonCrashLoop {
		t.Fatalf("reason = %s, want CrashLoop", res.Conditions[0].Reason)
	}
	f := dbprovision.View("prov_test", ms.events).Failure
	if f == nil || f.Cause == nil || f.Cause.Kind != dbprovision.CausePermission || f.LogTail == "" {
		t.Fatalf("failure = %+v, want permission cause and log tail", f)
	}
}

func TestProvision_FewRestartsKeepWaiting(t *testing.T) {
	rt := &crashRuntime{fakeRuntime: newFakeRuntime(), restarts: 1}
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	rt.containers[containerName("main")] = &docker.ContainerState{
		ID: "c1", Name: containerName("main"), Image: ImageRef(store.EngineRedis, "7"), Running: true,
		Created:  time.Now(),
		Networks: []docker.ContainerNetwork{{Name: "n", IPAddress: "10.0.0.5"}},
	}
	c, _ := gateFixture(t, rt, desired, WithReadyProbe(func(context.Context, string) error { return errors.New("refused") }))
	res, _ := c.Reconcile(context.Background())
	if res.Conditions[0].Reason != "Starting" {
		t.Fatalf("reason = %s, want Starting below the threshold", res.Conditions[0].Reason)
	}
}
