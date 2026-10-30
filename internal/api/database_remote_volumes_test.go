package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type volumeAgentRuntime struct {
	docker.Runtime
	removed []string
	failOn  string
}

func (v *volumeAgentRuntime) RemoveVolume(_ context.Context, name string) error {
	if name == v.failOn {
		return errors.New("volume in use")
	}
	v.removed = append(v.removed, name)
	return nil
}

func TestRemoveRemoteVolumes_RemovesAllAndReportsFailures(t *testing.T) {
	rt := &volumeAgentRuntime{}
	if err := removeRemoteVolumes(context.Background(), rt, databaseVolumeNames("far")); err != nil {
		t.Fatalf("removeRemoteVolumes: %v", err)
	}
	if len(rt.removed) != 3 {
		t.Fatalf("removed = %v, want data, certs and wal-archive volumes", rt.removed)
	}
	failing := &volumeAgentRuntime{failOn: "db-far-certs"}
	if err := removeRemoteVolumes(context.Background(), failing, databaseVolumeNames("far")); err == nil {
		t.Fatal("a failed removal must surface so the teardown job retries")
	}
	if err := removeRemoteVolumes(context.Background(), struct{ docker.Runtime }{}, []string{"x"}); err == nil {
		t.Fatal("an agent without volume removal must be an error, not a silent skip")
	}
}
