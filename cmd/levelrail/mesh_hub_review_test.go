package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/network"
)

type recordingRec struct{ got map[string]string }

func (r *recordingRec) SetObservedEndpoint(id, ep string) error {
	if ep == "" {
		delete(r.got, id)
		return nil
	}
	r.got[id] = ep
	return nil
}

func TestMeshHubSyncClearsEndpointWhenHostCleared(t *testing.T) {
	rec := &recordingRec{got: map[string]string{}}
	s := &meshHubSyncer{resolver: network.NewHubHostResolver(network.HubHostConfig{}), rec: rec, nodeID: "n1", logger: slog.New(slog.DiscardHandler)}
	s.Sync(context.Background(), "203.0.113.7")
	if rec.got["n1"] == "" {
		t.Fatal("endpoint not recorded")
	}
	s.Sync(context.Background(), "")
	if rec.got["n1"] != "" {
		t.Fatalf("endpoint still %q after the host was cleared", rec.got["n1"])
	}
}

func TestRunMeshHubRefreshSurvivesPanic(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := make(chan struct{}, 4)
	go runMeshHubRefresh(ctx, slog.New(slog.DiscardHandler), func() time.Duration { return time.Millisecond }, func(context.Context) {
		calls <- struct{}{}
		panic("boom")
	})
	for range 2 {
		select {
		case <-calls:
		case <-time.After(2 * time.Second):
			t.Fatal("refresh loop died after a panic")
		}
	}
}
