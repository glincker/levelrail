package main

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent"
	"github.com/GLINCKER/levelrail/internal/docker"
)

type pullingTransport struct {
	agent.Transport
	pulled []string
	fail   error
}

func (p *pullingTransport) PullImage(_ context.Context, ref string, _ *docker.RegistryAuth) error {
	p.pulled = append(p.pulled, ref)
	return p.fail
}

func TestWarmPullRemoteNode(t *testing.T) {
	reg := agent.NewRegistry()
	tr := &pullingTransport{}
	reg.Register("n1", tr)
	pull := warmPull(reg, nil)

	if err := pull(context.Background(), "n1", "node:22"); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if len(tr.pulled) != 1 || tr.pulled[0] != "node:22" {
		t.Fatalf("pulled = %v, want node:22 on the agent", tr.pulled)
	}

	reg.Register("n2", agent.NewLocal(nil))
	if err := pull(context.Background(), "n2", "node:22"); !errors.Is(err, agent.ErrImagePullUnsupported) {
		t.Fatalf("err = %v, want ErrImagePullUnsupported", err)
	}
	tr.fail = errors.New("denied")
	if err := pull(context.Background(), "n1", "node:22"); err == nil {
		t.Fatal("want the agent error")
	}
	if err := pull(context.Background(), "gone", "node:22"); err == nil {
		t.Fatal("want an error for an unregistered node")
	}
}
