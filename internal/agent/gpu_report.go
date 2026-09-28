package agent

import (
	"context"
	"time"

	"github.com/GLINCKER/levelrail/internal/agent/agentpb"
	"github.com/GLINCKER/levelrail/internal/gpu"
)

// gpuReportInterval is how often an agent re-reports its GPU snapshot so
// VRAM usage on the dashboard stays fresh.
const gpuReportInterval = 60 * time.Second

// GPUProbe returns the node's current GPU snapshot.
type GPUProbe func(ctx context.Context) gpu.Info

// WithGPUProbe makes RunSession report this node's GPUs to the control
// plane on connect and periodically after.
func WithGPUProbe(p GPUProbe) SessionOption {
	return func(c *sessionConfig) { c.gpuProbe = p }
}

// GPUSink receives a node's GPU report on the control plane.
type GPUSink interface {
	SetNodeGPU(ctx context.Context, nodeID string, info gpu.Info) error
}

// WithGPUSink persists GPU reports agents send up their Session stream.
func WithGPUSink(sink GPUSink) Option {
	return func(s *Server) { s.gpuSink = sink }
}

func gpuReportLoop(ctx context.Context, send func(*agentpb.AgentMessage), probe GPUProbe, interval time.Duration, done <-chan struct{}) {
	report := func() {
		send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_GpuReport{GpuReport: gpuInfoToPB(probe(ctx))}})
	}
	report()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			report()
		}
	}
}

func gpuInfoToPB(i gpu.Info) *agentpb.GPUReport {
	out := &agentpb.GPUReport{Present: i.Present, DriverVersion: i.DriverVersion, RuntimeInstalled: i.RuntimeInstalled}
	for _, d := range i.Devices {
		out.Devices = append(out.Devices, &agentpb.GPUDevice{
			Index:              int32(d.Index), //nolint:gosec // device indexes are tiny
			Uuid:               d.UUID,
			Name:               d.Name,
			VramTotalMib:       d.VRAMTotalMiB,
			VramUsedMib:        d.VRAMUsedMiB,
			UtilizationPercent: int32(d.UtilizationPercent), //nolint:gosec // 0-100
		})
	}
	return out
}

func gpuInfoFromPB(r *agentpb.GPUReport) gpu.Info {
	out := gpu.Info{Present: r.GetPresent(), DriverVersion: r.GetDriverVersion(), RuntimeInstalled: r.GetRuntimeInstalled()}
	for _, d := range r.GetDevices() {
		out.Devices = append(out.Devices, gpu.Device{
			Index:              int(d.GetIndex()),
			UUID:               d.GetUuid(),
			Name:               d.GetName(),
			VRAMTotalMiB:       d.GetVramTotalMib(),
			VRAMUsedMiB:        d.GetVramUsedMib(),
			UtilizationPercent: int(d.GetUtilizationPercent()),
		})
	}
	return out
}
