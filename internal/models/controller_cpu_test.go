package models

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

type fakeWeights struct {
	state  DownloadState
	dir    string
	specs  []WeightsSpec
	forget []string
}

func (f *fakeWeights) Ensure(s WeightsSpec) DownloadState {
	f.specs = append(f.specs, s)
	return f.state
}
func (f *fakeWeights) Dir(string) string { return f.dir }
func (f *fakeWeights) Forget(n string)   { f.forget = append(f.forget, n) }

func cpuModel() *store.Model {
	return &store.Model{Name: "local", Engine: EngineLlamaCpp, ModelRef: "ibm-granite/granite-4.1-3b-GGUF:Q4_K_M", Compute: ComputeCPU,
		CPUMilli: 2000, MemoryMiB: 4096, ContextLength: 4096, WeightsRepo: "ibm-granite/granite-4.1-3b-GGUF", WeightsFile: "granite-4.1-3b-Q4_K_M.gguf"}
}

var cpuNode = NodeInfo{BindIP: "127.0.0.1", DialHost: "127.0.0.1", Local: true}

func newCPUController(st *fakeStore, rt *fakeRuntime, w Weights, node NodeInfo) *Controller {
	return New(st.model.Name, st, fakeNodes{info: node}, rt, fakeProber{Status{Phase: PhaseLoading}},
		WithWeights(w), WithCPUPolicy(testPolicy()))
}

func TestController_CPU_RunsWithoutAGPUAndAppliesHardLimits(t *testing.T) {
	st := &fakeStore{model: cpuModel()}
	rt := newFakeRuntime()
	w := &fakeWeights{state: DownloadState{Phase: DLDone, Done: 2099501664, Total: 2099501664}, dir: "/var/lib/x/model-weights/local"}
	c := newCPUController(st, rt, w, cpuNode)

	res, err := c.Reconcile(context.Background())
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if cond := reason(t, res); cond.Reason != "Starting" {
		t.Fatalf("reason = %q (%s), want Starting with no GPU on the node", cond.Reason, cond.Message)
	}
	if len(rt.created) != 1 {
		t.Fatalf("created = %d", len(rt.created))
	}
	spec := rt.created[0]
	if spec.GPU != nil {
		t.Errorf("a CPU model must not request a GPU: %+v", spec.GPU)
	}
	if spec.Image != CPUImageDefault {
		t.Errorf("image = %q, want the CPU server image", spec.Image)
	}
	r := spec.Resources
	if r == nil || r.MemoryBytes != 4096<<20 || r.SwapMemoryBytes != r.MemoryBytes || r.NanoCPUs != 2_000_000_000 {
		t.Errorf("resources = %+v, want memory==swap (no swap) and 2 cpus", r)
	}
	if spec.CPUShares != 128 || spec.OomScoreAdj != 800 || spec.PidsLimit != 256 {
		t.Errorf("shares/oom/pids = %d/%d/%d", spec.CPUShares, spec.OomScoreAdj, spec.PidsLimit)
	}
	if len(spec.BindMounts) != 1 || !spec.BindMounts[0].ReadOnly || spec.BindMounts[0].HostPath != w.dir || len(spec.Volumes) != 0 {
		t.Errorf("mounts = %+v volumes = %+v", spec.BindMounts, spec.Volumes)
	}
	joined := strings.Join(spec.Command, " ")
	for _, want := range []string{"-m /models/granite-4.1-3b-Q4_K_M.gguf", "-t 2", "-np 1", "-c 4096", "-cram 204"} {
		if !strings.Contains(joined, want) {
			t.Errorf("command %q lacks %q", joined, want)
		}
	}
	if strings.Contains(joined, "-ngl") {
		t.Errorf("command must not offload to a GPU: %s", joined)
	}
	if _, ok := spec.Env["HF_TOKEN"]; ok {
		t.Error("the Hugging Face token must not enter a container that reads a local file")
	}
}

func TestController_CPU_ImageOverride(t *testing.T) {
	st := &fakeStore{model: cpuModel()}
	rt := newFakeRuntime()
	c := newCPUController(st, rt, &fakeWeights{state: DownloadState{Phase: DLDone, Total: 1}}, cpuNode)
	WithImages(map[string]string{ImageKeyLlamaCppCPU: "registry.local/llama:cpu", EngineLlamaCpp: "registry.local/llama:cuda"})(c)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rt.created[0].Image != "registry.local/llama:cpu" {
		t.Errorf("image = %q", rt.created[0].Image)
	}
}

func TestController_CPU_DownloadStatesBlockCreate(t *testing.T) {
	tests := []struct {
		name   string
		state  DownloadState
		reason string
		msg    string
	}{
		{"progress is shown", DownloadState{Phase: DLDownloading, Done: 1 << 30, Total: 2 << 30}, "Downloading", "50%"},
		{"verifying", DownloadState{Phase: DLVerifying, Done: 2 << 30, Total: 2 << 30}, "Downloading", "checksum"},
		{"gated is a clear failure", DownloadState{Phase: DLFailed, Err: &DownloadError{Code: DLGated, Message: "this repository is gated"}}, "DownloadFailed", "gated"},
		{"interrupted keeps the partial", DownloadState{Phase: DLFailed, Done: 1 << 30, Total: 2 << 30, Err: &DownloadError{Code: DLInterrupted, Message: "interrupted", Retryable: true}},
			"DownloadFailed", "kept, retrying"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := &fakeStore{model: cpuModel()}
			rt := newFakeRuntime()
			c := newCPUController(st, rt, &fakeWeights{state: tt.state}, cpuNode)
			res, err := c.Reconcile(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			cond := reason(t, res)
			if cond.Reason != tt.reason || !strings.Contains(cond.Message, tt.msg) {
				t.Errorf("condition = %+v", cond)
			}
			if len(rt.created) != 0 {
				t.Error("no container may be created before the weights are ready")
			}
		})
	}
}

func TestController_CPU_StartFailureKeepsWeightsAndRetryReusesThem(t *testing.T) {
	st := &fakeStore{model: cpuModel()}
	rt := newFakeRuntime()
	rt.createErr = errors.New("docker daemon unavailable")
	w := &fakeWeights{state: DownloadState{Phase: DLDone, Done: 5, Total: 5}}
	c := newCPUController(st, rt, w, cpuNode)

	res, err := c.Reconcile(context.Background())
	if err == nil {
		t.Fatal("create failure must surface")
	}
	cond := reason(t, res)
	if cond.Reason != "CreateFailed" || !strings.Contains(cond.Message, "weights are on disk") {
		t.Fatalf("condition = %+v", cond)
	}
	rt.createErr = nil
	res, err = c.Reconcile(context.Background())
	if err != nil || reason(t, res).Reason != "Starting" {
		t.Fatalf("retry = %+v, %v", res, err)
	}
	if len(w.specs) != 2 || w.specs[0].File != w.specs[1].File {
		t.Errorf("each pass asks for the same file, which the downloader serves from disk: %+v", w.specs)
	}
	if len(w.forget) != 0 {
		t.Error("a start failure must not discard downloaded weights")
	}
}

func TestController_CPU_RemoteNodeLetsTheEnginePull(t *testing.T) {
	st := &fakeStore{model: cpuModel()}
	rt := newFakeRuntime()
	w := &fakeWeights{state: DownloadState{Phase: DLDownloading}}
	remote := NodeInfo{BindIP: "10.0.0.2", DialHost: "10.0.0.2", Local: false}
	c := newCPUController(st, rt, w, remote)
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(w.specs) != 0 || len(rt.created) != 1 {
		t.Fatalf("remote node must not use the control plane downloader: specs=%d created=%d", len(w.specs), len(rt.created))
	}
	spec := rt.created[0]
	if len(spec.Volumes) != 1 || !strings.Contains(strings.Join(spec.Command, " "), "-hf ibm-granite/granite-4.1-3b-GGUF:Q4_K_M") {
		t.Errorf("volumes = %+v command = %v", spec.Volumes, spec.Command)
	}
	if spec.Resources == nil || spec.Resources.SwapMemoryBytes != spec.Resources.MemoryBytes {
		t.Errorf("remote CPU models keep the memory cap: %+v", spec.Resources)
	}
}

func TestController_CPU_MissingLimitsAreRefused(t *testing.T) {
	m := cpuModel()
	m.CPUMilli = 0
	st := &fakeStore{model: m}
	rt := newFakeRuntime()
	c := newCPUController(st, rt, &fakeWeights{state: DownloadState{Phase: DLDone}}, cpuNode)
	res, _ := c.Reconcile(context.Background())
	if reason(t, res).Reason != "CPULimitsMissing" || len(rt.created) != 0 {
		t.Fatalf("res = %+v created = %d", res, len(rt.created))
	}
}

func TestController_GPUModelsGetNoCPULimits(t *testing.T) {
	st := &fakeStore{model: testModel()}
	rt := newFakeRuntime()
	c := New("chat", st, fakeNodes{info: goodNode}, rt, fakeProber{Status{Phase: PhaseLoading}}, WithWeights(&fakeWeights{}))
	if _, err := c.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	spec := rt.created[0]
	if spec.Resources != nil || spec.CPUShares != 0 || spec.OomScoreAdj != 0 || spec.PidsLimit != 0 || len(spec.BindMounts) != 0 {
		t.Errorf("a GPU model must be created exactly as before: %+v", spec)
	}
	if spec.GPU == nil {
		t.Error("GPU model lost its GPU request")
	}
}

func TestController_GPUModelStillNeedsAGPU(t *testing.T) {
	st := &fakeStore{model: testModel()}
	c := New("chat", st, fakeNodes{info: NodeInfo{}}, newFakeRuntime(), fakeProber{})
	res, _ := c.Reconcile(context.Background())
	if reason(t, res).Reason != "NoGPUOnNode" {
		t.Fatalf("res = %+v", res)
	}
}

func TestController_ContainerNameStableForGPUAndChangesWithCPULimits(t *testing.T) {
	c := New("chat", &fakeStore{}, fakeNodes{}, newFakeRuntime(), fakeProber{})
	gpuModel := testModel()
	h := sha256.New()
	_, _ = fmt.Fprint(h, gpuModel.Engine, "|", gpuModel.ModelRef, "|", gpuModel.GPUCount, "|", strings.Join(gpuModel.GPUDeviceIDs, ","), "|",
		gpuModel.ContextLength, "|", gpuModel.Quantization, "|", gpuModel.RestartNonce, "|", "img", "|", false, "|", gpuModel.NodeID)
	if want := "platform-model-chat." + hex.EncodeToString(h.Sum(nil))[:8]; c.containerName(gpuModel, "img", false) != want {
		t.Fatalf("GPU container name changed, which would recreate every existing GPU model on upgrade")
	}
	a := cpuModel()
	b := cpuModel()
	b.MemoryMiB = 2048
	if c.containerName(a, "img", false) == c.containerName(b, "img", false) {
		t.Error("changing the memory cap must recreate the container")
	}
}

func TestController_CPUTeardownForgetsDownload(t *testing.T) {
	m := cpuModel()
	m.Deleting = true
	st := &fakeStore{model: m}
	w := &fakeWeights{}
	c := newCPUController(st, newFakeRuntime(), w, cpuNode)
	res, err := c.Reconcile(context.Background())
	if err != nil || reason(t, res).Reason != "Deleted" || len(w.forget) != 1 {
		t.Fatalf("res=%+v err=%v forget=%v", res, err, w.forget)
	}
}

func TestClassifyWriteErrDiskFull(t *testing.T) {
	e := classifyWriteErr(&os.PathError{Op: "write", Path: "x", Err: syscall.ENOSPC})
	if e.Code != DLDiskFull || !e.Retryable {
		t.Fatalf("err = %+v", e)
	}
	if classifyWriteErr(errors.New("boom")).Code == DLDiskFull {
		t.Error("only ENOSPC is disk full")
	}
}

func TestControllerResidencyIgnoresGPUWaitForCPUModels(t *testing.T) {
	m := cpuModel()
	m.Residency, m.ResidencyState = store.ResidencyOnDemand, store.ResidencyAsleep
	c := newCPUController(&fakeStore{model: m}, newFakeRuntime(), &fakeWeights{}, NodeInfo{GPU: gpu.Info{}})
	if blocked := c.waitForGPUWithSwap(context.Background(), m, NodeInfo{}); blocked != nil {
		t.Fatalf("a CPU model must never wait on VRAM: %+v", blocked)
	}
	_ = reconcile.Result{}
}
