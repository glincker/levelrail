package database

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dbprovision"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type volumeRuntime struct {
	*fakeRuntime
	vols    []docker.NamedVolume
	removed []string
}

func (v *volumeRuntime) ListNamedVolumes(context.Context) ([]docker.NamedVolume, error) {
	return v.vols, nil
}

func (v *volumeRuntime) RemoveVolume(_ context.Context, name string) error {
	v.removed = append(v.removed, name)
	return nil
}

func gateFixture(t *testing.T, rt docker.Runtime, desired *store.DesiredDatabase, opts ...Option) (*Controller, *memProvisionStore) {
	t.Helper()
	ms, rec := provisionFixture(t)
	opts = append(opts, WithProvisionRecorder(rec))
	return New("main", &fakeStore{db: desired}, rt, opts...), ms
}

func TestProvision_WaitsForDockerThenContinues(t *testing.T) {
	rt := newFakeRuntime()
	rt.inspectErr = errors.New("cannot connect to the docker daemon")
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	c, ms := gateFixture(t, rt, desired)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("waiting for Docker must not be an error, got %v", err)
	}
	if res.RequeueAfter <= 0 || res.Conditions[0].Reason != "WaitingForDocker" {
		t.Fatalf("result = %+v, want a requeue with WaitingForDocker", res)
	}
	a := dbprovision.View("prov_test", ms.events)
	if statusOf(a, dbprovision.StepDockerCheck) != dbprovision.StatusStarted || a.Failure != nil {
		t.Fatalf("docker_check = %+v failure = %+v, want started and no failure", a.Steps, a.Failure)
	}
	if rt.createCalls != 0 {
		t.Fatalf("created %d containers while Docker was down", rt.createCalls)
	}

	rt.inspectErr = nil
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	a = dbprovision.View("prov_test", ms.events)
	if a.Status != dbprovision.AttemptReady || rt.createCalls != 1 {
		t.Fatalf("status = %s creates = %d, want ready after Docker returned", a.Status, rt.createCalls)
	}
	if got := statusOf(a, dbprovision.StepApplySettings); got != dbprovision.StatusDone {
		t.Fatalf("apply_settings = %s, want done", got)
	}
}

func TestProvision_LeftoverVolumeStopsWithTypedFix(t *testing.T) {
	cases := []struct {
		name, choice string
		mounted      bool
		wantCreate   int
		wantRemoved  int
		wantStop     bool
	}{
		{name: "no choice stops", wantStop: true},
		{name: "reuse attaches", choice: "reuse", wantCreate: 1},
		{name: "discard removes then creates", choice: "discard", wantCreate: 1, wantRemoved: 1},
		{name: "discard of a mounted volume fails", choice: "discard", mounted: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &volumeRuntime{fakeRuntime: newFakeRuntime(), vols: []docker.NamedVolume{{Name: "db-main-data", SizeBytes: 4096, Mounted: tc.mounted}}}
			desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7", ExistingVolume: tc.choice}
			c, ms := gateFixture(t, rt, desired)
			if _, err := c.Reconcile(context.Background()); err != nil {
				t.Fatal(err)
			}
			if rt.createCalls != tc.wantCreate || len(rt.removed) != tc.wantRemoved {
				t.Fatalf("creates = %d removed = %v, want %d and %d", rt.createCalls, rt.removed, tc.wantCreate, tc.wantRemoved)
			}
			a := dbprovision.View("prov_test", ms.events)
			if tc.wantStop {
				f := a.Failure
				if f == nil || f.Step != dbprovision.StepCreateVolume || f.Volume == nil || f.Volume.Name != "db-main-data" || f.Volume.SizeBytes != 4096 || len(f.Fixes) != 3 {
					t.Fatalf("failure = %+v, want a typed create_volume stop with volume and three fixes", f)
				}
				if !f.Fixes[1].Confirm {
					t.Fatalf("removing the volume must need confirmation: %+v", f.Fixes)
				}
			}
		})
	}
}

func TestProvision_FailureKeepsSettingsAndRetryResolvesStop(t *testing.T) {
	rt := &volumeRuntime{fakeRuntime: newFakeRuntime(), vols: []docker.NamedVolume{{Name: "db-main-data"}}}
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7", Resources: &store.ServiceResources{MemoryBytes: 256 << 20}}
	c, _ := gateFixture(t, rt, desired)
	_, _ = c.Reconcile(context.Background())
	if desired.Resources == nil || desired.Resources.MemoryBytes != 256<<20 || desired.Version != "7" {
		t.Fatalf("stored settings changed by a failed attempt: %+v", desired)
	}
	desired.ExistingVolume = "reuse"
	if _, err := c.Reconcile(context.Background()); err != nil || rt.createCalls != 1 {
		t.Fatalf("err = %v creates = %d, want the retry to create the container", err, rt.createCalls)
	}
}

func TestProvision_ReadyMeansAcceptingConnections(t *testing.T) {
	rt := newFakeRuntime()
	desired := &store.DesiredDatabase{Name: "main", Engine: store.EngineRedis, Version: "7"}
	rt.containers[containerName("main")] = &docker.ContainerState{
		ID: "c1", Name: containerName("main"), Image: ImageRef(store.EngineRedis, "7"), Running: true,
		Networks: []docker.ContainerNetwork{{Name: "n", IPAddress: "10.0.0.5"}},
	}
	up := false
	c, ms := gateFixture(t, rt, desired, WithReadyProbe(func(_ context.Context, addr string) error {
		if addr != "10.0.0.5:6379" {
			t.Errorf("probed %s, want the container address and engine port", addr)
		}
		if !up {
			return errors.New("connection refused")
		}
		return nil
	}))

	res, _ := c.Reconcile(context.Background())
	if res.RequeueAfter <= 0 || res.Conditions[0].Status != reconcile.ConditionFalse {
		t.Fatalf("result = %+v, want not ready and a requeue while the port is closed", res)
	}
	if got := statusOf(dbprovision.View("prov_test", ms.events), dbprovision.StepWaitReady); got != dbprovision.StatusStarted {
		t.Fatalf("wait_ready = %s, want started", got)
	}
	up = true
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := dbprovision.View("prov_test", ms.events).Status; got != dbprovision.AttemptReady {
		t.Fatalf("status = %s, want ready once the port accepts connections", got)
	}
}
