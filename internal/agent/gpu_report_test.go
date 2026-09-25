package agent

import (
	"context"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/gpu"
)

type recordingGPUSink struct {
	got chan gpu.Info
	id  chan string
}

func (r recordingGPUSink) SetNodeGPU(_ context.Context, nodeID string, info gpu.Info) error {
	r.id <- nodeID
	r.got <- info
	return nil
}

func TestGPUInfoPBRoundTrip(t *testing.T) {
	tests := []gpu.Info{
		{},
		{Present: true, DriverVersion: "550", RuntimeInstalled: true, Devices: []gpu.Device{
			{Index: 0, UUID: "GPU-a", Name: "A100", VRAMTotalMiB: 40960, VRAMUsedMiB: 5, UtilizationPercent: 9},
			{Index: 1, UUID: "GPU-b", Name: "A100", VRAMTotalMiB: 40960},
		}},
	}
	for _, in := range tests {
		if got := gpuInfoFromPB(gpuInfoToPB(in)); !reflect.DeepEqual(got, in) {
			t.Errorf("round trip = %+v, want %+v", got, in)
		}
	}
}

func TestServer_GPUReportReachesSink(t *testing.T) {
	sink := recordingGPUSink{got: make(chan gpu.Info, 1), id: make(chan string, 1)}
	s := &Server{gpuSink: sink, logger: slog.Default()}
	stream := newFakeSessionStream()
	m := newMuxWithHandlers(stream, s.onGPUReport("node-1"), nil)
	defer close(stream.recv)

	want := gpu.Info{Present: true, DriverVersion: "550", Devices: []gpu.Device{{Index: 0, Name: "L4", VRAMTotalMiB: 24576}}}
	stream.recv <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_GpuReport{GpuReport: gpuInfoToPB(want)}}

	select {
	case id := <-sink.id:
		if id != "node-1" {
			t.Errorf("node id = %q", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for gpu report")
	}
	if got := <-sink.got; !reflect.DeepEqual(got, want) {
		t.Errorf("info = %+v, want %+v", got, want)
	}
	_ = m
}

func TestServer_NoGPUSinkIgnoresReports(t *testing.T) {
	s := &Server{logger: slog.Default()}
	if s.onGPUReport("n") != nil {
		t.Error("onGPUReport without a sink must be nil")
	}
}

func TestGPUReportLoop_ReportsImmediatelyAndPeriodically(t *testing.T) {
	sent := make(chan *agentpb.AgentMessage, 8)
	done := make(chan struct{})
	defer close(done)
	probe := func(context.Context) gpu.Info { return gpu.Info{Present: true} }
	go gpuReportLoop(context.Background(), func(m *agentpb.AgentMessage) { sent <- m }, probe, 10*time.Millisecond, done)
	for i := 0; i < 2; i++ {
		select {
		case m := <-sent:
			if !m.GetGpuReport().GetPresent() {
				t.Errorf("report %d not present", i)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for report %d", i)
		}
	}
}
