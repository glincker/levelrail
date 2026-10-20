package api

import (
	"context"
	"fmt"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/models"
)

// GPUHostDiagnoser inspects the control plane host's GPU stack in detail:
// driver, toolkit, CDI specs and how containers can attach GPUs.
type GPUHostDiagnoser func(ctx context.Context) gpu.HostDiagnosis

// WithGPUHostDiagnoser adds the detailed host GPU checks to the doctor
// report. Without it the doctor shows only what nodes report.
func WithGPUHostDiagnoser(d GPUHostDiagnoser) Option {
	return func(rt *Router) { rt.gpuHostDiagnoser = d }
}

// doctorHostGPUChecks turns the local host's diagnosis into doctor checks.
// Remote nodes report only driver, runtime and per-GPU memory, so the
// toolkit and CDI detail is for the control plane host alone.
func (rt *Router) doctorHostGPUChecks(ctx context.Context, node string) []doctorCheckResource {
	diag := rt.gpuHostDiagnoser(ctx)
	out := make([]doctorCheckResource, 0, len(diag.Findings))
	for _, f := range diag.Findings {
		c := doctorCheckResource{Code: "gpu:" + node + ":" + f.Code, Name: f.Name + " on " + node, Status: doctorStatusFromGPU(f.Status),
			Message: f.Message, Fix: f.Fix}
		if f.Status != gpu.StatusOK {
			c.DocsPath = "/ai-models#gpu-attach-cdi-and-legacy"
		}
		out = append(out, c)
	}
	return out
}

func doctorStatusFromGPU(s string) string {
	switch s {
	case gpu.StatusFail:
		return doctorStatusFail
	case gpu.StatusWarn:
		return doctorStatusWarn
	}
	return doctorStatusOK
}

// gpuMemoryChecks lists per-GPU memory for a node the control plane cannot
// inspect directly.
func gpuMemoryChecks(n models.GPUNode) []doctorCheckResource {
	out := make([]doctorCheckResource, 0, len(n.Info.Devices))
	for _, d := range n.Info.Devices {
		out = append(out, doctorCheckResource{
			Code: fmt.Sprintf("gpu:%s:gpu%d", n.Name, d.Index), Name: fmt.Sprintf("GPU %d memory on %s", d.Index, n.Name), Status: doctorStatusOK,
			Message: fmt.Sprintf("%s: %.1f GiB used of %.1f GiB", d.Name, float64(d.VRAMUsedMiB)/1024, float64(d.VRAMTotalMiB)/1024),
		})
	}
	return out
}
