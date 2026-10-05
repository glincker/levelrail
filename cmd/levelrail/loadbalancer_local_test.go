package main

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type localRuntimeMarker struct{ docker.Runtime }

func TestLBNodeUpstreamsLocalNodeDialsLoopback(t *testing.T) {
	local := localRuntimeMarker{}
	r := lbNodeUpstreams{local: local, localNodeID: "local_abc"}

	rt, host, err := r.UpstreamHost(context.Background(), "local_abc")
	if err != nil {
		t.Fatalf("UpstreamHost for the local node: %v", err)
	}
	if rt != docker.Runtime(local) {
		t.Errorf("runtime = %v, want the local runtime", rt)
	}
	if host != "127.0.0.1" {
		t.Errorf("host = %q, want 127.0.0.1", host)
	}
}
