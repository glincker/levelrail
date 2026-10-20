package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeNodeGPU struct {
	info gpu.Info
	err  error
}

func (f fakeNodeGPU) NodeGPU(context.Context, string) (gpu.Info, error) { return f.info, f.err }

func TestController_Reconcile_GPUPlacement(t *testing.T) {
	gpuSvc := func() *store.DesiredService {
		return &store.DesiredService{
			Name: "llm", Image: "img:v1", Port: 80,
			Resources: &store.ServiceResources{GPU: &store.ServiceGPU{Count: 2}},
		}
	}
	tests := []struct {
		name        string
		svc         *store.DesiredService
		checker     NodeGPUChecker
		wantReason  string
		wantCreates int
		wantGPU     *docker.GPURequest
	}{
		{name: "no checker configured still creates", svc: gpuSvc(), checker: nil, wantReason: "Deployed", wantCreates: 1, wantGPU: &docker.GPURequest{Count: 2}},
		{name: "node has no gpu", svc: gpuSvc(), checker: fakeNodeGPU{}, wantReason: "NoGPUOnNode"},
		{name: "runtime missing", svc: gpuSvc(), checker: fakeNodeGPU{info: gpu.Info{Present: true}}, wantReason: "GPURuntimeMissing"},
		{name: "check fails", svc: gpuSvc(), checker: fakeNodeGPU{err: errors.New("db down")}, wantReason: "GPUCheckFailed"},
		{name: "usable gpu node", svc: gpuSvc(), checker: fakeNodeGPU{info: gpu.Info{Present: true, RuntimeInstalled: true}}, wantReason: "Deployed", wantCreates: 1, wantGPU: &docker.GPURequest{Count: 2}},
		{name: "non gpu service ignores checker", svc: &store.DesiredService{Name: "llm", Image: "img:v1", Port: 80}, checker: fakeNodeGPU{}, wantReason: "Deployed", wantCreates: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newFakeRuntime(0)
			var opts []Option
			if tt.checker != nil {
				opts = append(opts, WithNodeGPU(tt.checker))
			}
			c := New("llm", &fakeStore{svc: tt.svc}, rt, opts...)
			result, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatalf("Reconcile() error = %v", err)
			}
			cond := conditionOf(t, result)
			if cond.Reason != tt.wantReason {
				t.Errorf("reason = %q, want %q (%+v)", cond.Reason, tt.wantReason, cond)
			}
			if tt.wantReason != "Deployed" && cond.Status != reconcile.ConditionFalse {
				t.Errorf("status = %v, want False", cond.Status)
			}
			if rt.createCalls != tt.wantCreates {
				t.Errorf("createCalls = %d, want %d", rt.createCalls, tt.wantCreates)
			}
			if tt.wantCreates > 0 && !reflect.DeepEqual(rt.lastCreateSpec.GPU, tt.wantGPU) {
				t.Errorf("ContainerSpec.GPU = %+v, want %+v", rt.lastCreateSpec.GPU, tt.wantGPU)
			}
		})
	}
}
