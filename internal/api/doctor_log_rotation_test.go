package api

import (
	"context"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

type fakeLogPinger struct {
	list []docker.ContainerState
	info map[string]docker.LogRotationInfo
}

func (fakeLogPinger) Ping(context.Context) error { return nil }
func (f fakeLogPinger) ListByPrefix(context.Context, string) ([]docker.ContainerState, error) {
	return f.list, nil
}
func (f fakeLogPinger) InspectLogRotation(_ context.Context, id string) (docker.LogRotationInfo, error) {
	return f.info[id], nil
}

func TestDoctorCheckLogRotation(t *testing.T) {
	list := []docker.ContainerState{
		{ID: "a", Name: "web", Running: true},
		{ID: "b", Name: "db", Running: true},
		{ID: "c", Name: "off", Running: false},
	}
	rt := &Router{dockerPinger: fakeLogPinger{list: list, info: map[string]docker.LogRotationInfo{
		"a": {Driver: "json-file", MaxSize: "10m", MaxFile: 3},
		"b": {Driver: "json-file"},
	}}}
	got := rt.doctorCheckLogRotation(context.Background())
	if got.Status != doctorStatusWarn || got.Fix == "" {
		t.Fatalf("unbounded log must warn: %v %s", got.Status, got.Message)
	}
	rt = &Router{dockerPinger: fakeLogPinger{list: list, info: map[string]docker.LogRotationInfo{
		"a": {Driver: "local"}, "b": {Driver: "json-file", MaxSize: "5m"},
	}}}
	if got := rt.doctorCheckLogRotation(context.Background()); got.Status != doctorStatusOK {
		t.Fatalf("rotated logs: %v %s", got.Status, got.Message)
	}
	if got := (&Router{}).doctorCheckLogRotation(context.Background()); got.Status != doctorStatusUnknown {
		t.Fatalf("no client: %v", got.Status)
	}
}
