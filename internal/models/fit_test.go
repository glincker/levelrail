package models

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/gpu"
	"github.com/GLINCKER/levelrail/internal/reconcile"
	"github.com/GLINCKER/levelrail/internal/store"
)

const gibBytes = int64(1) << 30

func testFitParams() FitParams {
	return FitParams{GPUOverheadMiB: 512, VLLMExtraMiB: 1024, DefaultContext: 8192, VLLMUtilization: 0.9}
}

func testFitConfig() FitConfig { return FitConfig{FitPercent: 90} }

func TestParseParamsBillions(t *testing.T) {
	tests := []struct {
		ref  string
		want float64
		ok   bool
	}{
		{"llama3.1:8b", 8, true},
		{"llama3.1:70b-instruct-q4_K_M", 70, true},
		{"qwen2.5:0.5b", 0.5, true},
		{"org/Llama-3.2-3B-Instruct-GGUF:Q4_K_M", 3, true},
		{"mixtral:8x7b", 56, true},
		{"org/Mixtral-8x7B-v0.1-GGUF", 56, true},
		{"mistral", 0, false},
		{"phi3:mini", 0, false},
		{"org/model-Q4_K_M-GGUF", 0, false},
	}
	for _, tc := range tests {
		got, ok := ParseParamsBillions(tc.ref)
		if ok != tc.ok || got != tc.want {
			t.Errorf("ParseParamsBillions(%q) = %v, %v; want %v, %v", tc.ref, got, ok, tc.want, tc.ok)
		}
	}
}

func TestEstimateWeightBytes(t *testing.T) {
	tests := []struct {
		name               string
		engine, ref, quant string
		want               float64
	}{
		{"ollama default is about 4.85 bits", EngineOllama, "llama3.1:8b", "", 8e9 * 4.85 / 8},
		{"ollama tag quant", EngineOllama, "llama3.1:8b-instruct-q8_0", "", 8e9 * 8.5 / 8},
		{"llamacpp suffix quant", EngineLlamaCpp, "org/m-8B-GGUF:Q6_K", "", 8e9 * 6.6 / 8},
		{"vllm defaults to bf16", EngineVLLM, "org/Llama-3.1-8B-Instruct", "", 16e9},
		{"vllm awq quantization", EngineVLLM, "org/Llama-3.1-8B-Instruct", "awq", 8e9 * 4.5 / 8},
		{"vllm awq in the repo name", EngineVLLM, "org/Llama-3.1-8B-Instruct-AWQ", "", 8e9 * 4.5 / 8},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := EstimateWeightBytes(tc.engine, tc.ref, tc.quant)
			if !ok || float64(got) != tc.want {
				t.Fatalf("got %d, %v; want %v", got, ok, tc.want)
			}
		})
	}
	if _, ok := EstimateWeightBytes(EngineOllama, "mistral", ""); ok {
		t.Fatal("a name with no size must not be guessed")
	}
}

func TestFitCheck(t *testing.T) {
	tests := []struct {
		name        string
		in          FitInput
		wantVerdict string
		wantSource  string
		wantInText  string
	}{
		{
			name:        "8b q4 on a 24 GiB card fits",
			in:          FitInput{Engine: EngineOllama, ModelRef: "llama3.1:8b", GPUs: 1, FreeBytes: 24 * gibBytes, MinFreeBytes: 24 * gibBytes, MinTotalBytes: 24 * gibBytes},
			wantVerdict: FitFits, wantSource: WeightsEstimated, wantInText: "weights 4.5 GiB",
		},
		{
			name:        "70b q4 on one 24 GiB card does not fit",
			in:          FitInput{Engine: EngineOllama, ModelRef: "llama3.1:70b", GPUs: 1, FreeBytes: 24 * gibBytes, MinFreeBytes: 24 * gibBytes, MinTotalBytes: 24 * gibBytes},
			wantVerdict: FitWontFit, wantSource: WeightsEstimated,
		},
		{
			name:        "long context turns fits into tight",
			in:          FitInput{Engine: EngineOllama, ModelRef: "llama3.1:8b", ContextLength: 140000, GPUs: 1, FreeBytes: 24 * gibBytes, MinFreeBytes: 24 * gibBytes, MinTotalBytes: 24 * gibBytes},
			wantVerdict: FitTight, wantSource: WeightsEstimated,
		},
		{
			name:        "exact weight size wins over the name",
			in:          FitInput{Engine: EngineLlamaCpp, ModelRef: "org/m-8B-GGUF", WeightsBytes: 30 * gibBytes, GPUs: 1, FreeBytes: 24 * gibBytes, MinFreeBytes: 24 * gibBytes, MinTotalBytes: 24 * gibBytes},
			wantVerdict: FitWontFit, wantSource: WeightsExact,
		},
		{
			name:        "size not in the name is unknown, never guessed",
			in:          FitInput{Engine: EngineOllama, ModelRef: "mistral", GPUs: 1, FreeBytes: 24 * gibBytes, MinFreeBytes: 24 * gibBytes, MinTotalBytes: 24 * gibBytes},
			wantVerdict: FitUnknown, wantSource: WeightsUnknown,
		},
		{
			name:        "no GPU memory report is unknown",
			in:          FitInput{Engine: EngineOllama, ModelRef: "llama3.1:8b", GPUs: 0, FreeBytes: -1, MinFreeBytes: -1},
			wantVerdict: FitUnknown, wantSource: WeightsEstimated,
		},
		{
			name:        "vllm refuses a GPU another workload already holds",
			in:          FitInput{Engine: EngineVLLM, ModelRef: "org/Llama-3.1-8B-Instruct", GPUs: 1, FreeBytes: 20 * gibBytes, MinFreeBytes: 20 * gibBytes, MinTotalBytes: 24 * gibBytes},
			wantVerdict: FitWontFit, wantSource: WeightsEstimated, wantInText: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := testFitConfig().Check(testFitParams(), tc.in)
			if got.Verdict != tc.wantVerdict || got.WeightsSource != tc.wantSource {
				t.Fatalf("verdict/source = %s/%s, want %s/%s (%+v)", got.Verdict, got.WeightsSource, tc.wantVerdict, tc.wantSource, got)
			}
			if !strings.Contains(got.Arithmetic, tc.wantInText) {
				t.Errorf("arithmetic %q lacks %q", got.Arithmetic, tc.wantInText)
			}
			if got.Verdict == FitWontFit && len(got.Suggestions) == 0 {
				t.Error("a failing check should say what to change")
			}
		})
	}
}

func TestFitCheckVLLMReasonMentionsUtilization(t *testing.T) {
	got := testFitConfig().Check(testFitParams(), FitInput{Engine: EngineVLLM, ModelRef: "org/Llama-3.1-8B-Instruct", GPUs: 1,
		FreeBytes: 20 * gibBytes, MinFreeBytes: 20 * gibBytes, MinTotalBytes: 24 * gibBytes})
	if !strings.Contains(got.Reason, "90%") {
		t.Fatalf("reason = %q", got.Reason)
	}
}

func TestServiceFitCheckRanksNodesAndReservesPendingModels(t *testing.T) {
	ctx := context.Background()
	svc, db := newSvc(t, nil)
	addNode(t, db, "big", gpuInfo(1, true), true)
	small := gpu.Info{Present: true, RuntimeInstalled: true, Devices: []gpu.Device{{Index: 0, UUID: "GPU-s", VRAMTotalMiB: 8192}}}
	addNode(t, db, "small", small, true)
	addNode(t, db, "cordoned", gpuInfo(1, true), false)

	req := FitRequest{Engine: EngineOllama, ModelRef: "llama3.1:8b"}
	rep, err := svc.FitCheck(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Nodes) != 3 || rep.Note == "" {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Nodes[0].Name != "big" || rep.Nodes[0].Verdict != FitFits {
		t.Errorf("best node = %+v", rep.Nodes[0])
	}
	if last := rep.Nodes[len(rep.Nodes)-1]; last.Name != "cordoned" || last.Eligible {
		t.Errorf("ineligible node must rank last: %+v", last)
	}

	// A model that is placed but not loaded yet holds its estimated VRAM.
	if err := db.SaveModel(ctx, store.Model{Name: "pending", Engine: EngineOllama, ModelRef: "llama3.1:70b", NodeID: "big", GPUCount: -1}); err != nil {
		t.Fatal(err)
	}
	one := "big"
	rep, err = svc.FitCheck(ctx, FitRequest{Engine: EngineOllama, ModelRef: "llama3.1:8b", NodeID: &one})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Nodes) != 1 || rep.Nodes[0].ReservedBytes == 0 || rep.Nodes[0].Verdict != FitWontFit {
		t.Fatalf("pending 70b must crowd out the 8b on a 24 GiB node: %+v", rep.Nodes)
	}

	if _, err := svc.FitCheck(ctx, FitRequest{Engine: "nope", ModelRef: "x"}); err == nil {
		t.Fatal("unknown engine must be rejected")
	}
}

func TestServiceModelFitCountsOwnLoadedVRAM(t *testing.T) {
	ctx := context.Background()
	svc, db := newSvc(t, nil)
	info := gpuInfo(1, true)
	info.Devices[0].VRAMUsedMiB = 22 * 1024
	addNode(t, db, "n1", info, true)
	if err := db.SaveModel(ctx, store.Model{Name: "chat", Engine: EngineOllama, ModelRef: "llama3.1:8b", NodeID: "n1", GPUCount: -1}); err != nil {
		t.Fatal(err)
	}
	rep, err := svc.ModelFit(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.Nodes[0]; !got.Current || got.Verdict == FitFits {
		t.Fatalf("not loaded: only 2 GiB is free, so it must not fit: %+v", got)
	}
	if err := db.UpsertConditions(ctx, ControllerName("chat"), []reconcile.Condition{{Type: "Ready", Status: reconcile.ConditionTrue, Reason: "ModelLoaded"}}); err != nil {
		t.Fatal(err)
	}
	rep, err = svc.ModelFit(ctx, "chat")
	if err != nil {
		t.Fatal(err)
	}
	if got := rep.Nodes[0]; got.Verdict != FitFits {
		t.Fatalf("loaded: its own VRAM is added back, so it fits: %+v", got)
	}
}
