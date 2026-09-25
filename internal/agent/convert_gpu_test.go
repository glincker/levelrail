package agent

import (
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
)

func TestPortBindingHostIPRoundTrip(t *testing.T) {
	in := docker.PortBinding{ContainerPort: 8000, HostPort: 9000, Protocol: "tcp", HostIP: "10.99.0.2"}
	if got := portBindingFromPB(portBindingToPB(in)); got != in {
		t.Errorf("round trip = %+v, want %+v", got, in)
	}
}

func TestContainerSpecGPUCommandRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		gpu     *docker.GPURequest
		command []string
	}{
		{name: "none"},
		{name: "all gpus", gpu: &docker.GPURequest{Count: -1}},
		{name: "device ids and command", gpu: &docker.GPURequest{DeviceIDs: []string{"0", "2"}}, command: []string{"--model", "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			back := containerSpecFromPB(containerSpecToPB(docker.ContainerSpec{Name: "m", Image: "i", GPU: tt.gpu, Command: tt.command, ShmSizeBytes: 1 << 30}))
			if back.ShmSizeBytes != 1<<30 {
				t.Errorf("ShmSizeBytes = %d", back.ShmSizeBytes)
			}
			if !reflect.DeepEqual(back.GPU, tt.gpu) {
				t.Errorf("GPU = %+v, want %+v", back.GPU, tt.gpu)
			}
			if !reflect.DeepEqual(back.Command, tt.command) {
				t.Errorf("Command = %+v, want %+v", back.Command, tt.command)
			}
		})
	}
}
