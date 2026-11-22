package application

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type prePullRuntime struct {
	docker.Runtime
	pulled []string
}

func (r *prePullRuntime) PullImage(_ context.Context, ref string, _ *docker.RegistryAuth) error {
	r.pulled = append(r.pulled, ref)
	return nil
}

func TestStartPrePull(t *testing.T) {
	rt := &prePullRuntime{}
	c := &Controller{runtime: rt}
	c.startPrePull(context.Background(), docker.ContainerSpec{Image: "reg/app:1"})()
	if len(rt.pulled) != 1 || rt.pulled[0] != "reg/app:1" {
		t.Fatalf("pulled = %v", rt.pulled)
	}
	c.startPrePull(context.Background(), docker.ContainerSpec{Image: "reg/app:2", RegistryAuth: &docker.RegistryAuth{Username: "u", Password: "p"}})()
	if len(rt.pulled) != 1 {
		t.Fatalf("basic-auth pull must stay with Create, pulled = %v", rt.pulled)
	}
}
