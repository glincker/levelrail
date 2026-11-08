package docker

import (
	"testing"

	"github.com/docker/docker/api/types/container"
)

func TestLogRotationApply(t *testing.T) {
	hc := &container.HostConfig{}
	applyLogRotation(hc, LogRotation{})
	if hc.LogConfig.Type != "" {
		t.Fatalf("zero rotation must change nothing, got %+v", hc.LogConfig)
	}
	applyLogRotation(hc, LogRotation{MaxSizeMB: 10, MaxFiles: 3})
	if hc.LogConfig.Config["max-size"] != "10m" || hc.LogConfig.Config["max-file"] != "3" {
		t.Fatalf("log config = %+v", hc.LogConfig)
	}
}

func TestLogRotationSource(t *testing.T) {
	t.Cleanup(func() { SetLogRotationSource(nil) })
	SetLogRotationSource(func() LogRotation { return LogRotation{MaxSizeMB: 5, MaxFiles: 2} })
	hc := buildHostConfig(ContainerSpec{}, nil)
	if hc.LogConfig.Config["max-size"] != "5m" {
		t.Fatalf("source not applied: %+v", hc.LogConfig)
	}
}
