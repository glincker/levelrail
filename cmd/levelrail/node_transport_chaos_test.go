package main

import (
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/agent"
)

// A severed agent drops out of the registry, so its services are skipped (not
// torn down) each pass; when the agent reconnects the same node resolves again.
func TestChaosPack_AgentSeveredThenReconnected_TransportSkippedThenResolves(t *testing.T) {
	registry := agent.NewRegistry()
	local := &fakeInspectRuntime{}
	first := agent.NewLocal(&fakeInspectRuntime{})
	registry.Register("remote-1", first)

	if _, err := resolveNodeTransport(local, registry, "remote-1"); err != nil {
		t.Fatalf("resolve while connected error = %v", err)
	}

	registry.Unregister("remote-1")
	for i := 0; i < 3; i++ {
		_, err := resolveNodeTransport(local, registry, "remote-1")
		if !errors.Is(err, agent.ErrNodeNotRegistered) {
			t.Fatalf("pass %d resolve error = %v, want ErrNodeNotRegistered so the pass skips the service", i, err)
		}
	}
	if rt, err := resolveNodeTransport(local, registry, ""); err != nil || rt != local {
		t.Fatalf("local node resolve = %v, %v, want the local runtime unaffected by the severed agent", rt, err)
	}

	second := agent.NewLocal(&fakeInspectRuntime{})
	registry.Register("remote-1", second)
	got, err := resolveNodeTransport(local, registry, "remote-1")
	if err != nil {
		t.Fatalf("resolve after reconnect error = %v", err)
	}
	if got != second {
		t.Fatal("resolve after reconnect did not return the new session's transport")
	}
}
