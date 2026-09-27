package models

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// FitNoteVRAM states the limits of the per-node VRAM check.
const FitNoteVRAM = "This is an estimate, not a guarantee: weights are sized from the model name and quantization (exact when the Hugging Face file sizes are known), " +
	"the KV cache is approximated from the parameter count and context length, and engine overhead is a flat allowance. " +
	"Real usage depends on the engine version, batch size and driver."

// Source of the weight size behind a fit result.
const (
	WeightsExact     = "exact"
	WeightsEstimated = "estimated"
	WeightsUnknown   = "unknown"
)

// FitParams are the VRAM estimation knobs, all environment driven.
type FitParams struct {
	// GPUOverheadMiB is reserved per GPU for the CUDA context and buffers.
	GPUOverheadMiB int64
	// VLLMExtraMiB is added per GPU for vLLM's CUDA graphs and torch context.
	VLLMExtraMiB int64
	// DefaultContext is assumed when a model sets no context length.
	DefaultContext int
	// VLLMUtilization is the share of each GPU vLLM pre-allocates.
	VLLMUtilization float64
	// KVKiBPerToken overrides the parameter-count heuristic when positive.
	KVKiBPerToken float64
}

// LoadFitParams reads APP_MODEL_FIT_GPU_OVERHEAD_MIB, APP_MODEL_FIT_VLLM_EXTRA_MIB,
// APP_MODEL_FIT_DEFAULT_CONTEXT, APP_MODEL_VLLM_GPU_UTILIZATION and
// APP_MODEL_FIT_KV_KIB_PER_TOKEN.
func LoadFitParams() FitParams {
	return FitParams{
		GPUOverheadMiB:  int64(envFloat("APP_MODEL_FIT_GPU_OVERHEAD_MIB", 512)),
		VLLMExtraMiB:    int64(envFloat("APP_MODEL_FIT_VLLM_EXTRA_MIB", 1024)),
		DefaultContext:  int(envFloat("APP_MODEL_FIT_DEFAULT_CONTEXT", 8192)),
		VLLMUtilization: envFloat("APP_MODEL_VLLM_GPU_UTILIZATION", 0.9),
		KVKiBPerToken:   envFloat("APP_MODEL_FIT_KV_KIB_PER_TOKEN", 0),
	}
}

var (
	paramsRe   = regexp.MustCompile(`(?i)(?:^|[^0-9a-z.])(?:(\d+)x)?(\d+(?:\.\d+)?)b(?:[^a-z]|$)`)
	refQuantRe = regexp.MustCompile(`(?i)(?:^|[^a-z0-9])((?:UD-)?I?Q[1-8](?:_[A-Z0-9]+)*|BF16|F16|F32|FP16|FP8|AWQ|GPTQ)(?:[^a-z0-9]|$)`)
)

var bitsPerWeight = map[string]float64{
	"F32": 32, "F16": 16, "FP16": 16, "BF16": 16, "FP8": 8, "AWQ": 4.5, "GPTQ": 4.5,
	"Q8_0": 8.5, "Q6_K": 6.6, "Q5_K_M": 5.7, "Q5_K_S": 5.5, "Q5_0": 5.5, "Q4_K_M": 4.85, "Q4_K_S": 4.6,
	"Q4_0": 4.5, "Q3_K_M": 3.9, "Q3_K_S": 3.5, "Q2_K": 3.4, "IQ4_XS": 4.3, "IQ3_M": 3.7, "IQ2_M": 2.7,
}

const defaultQuantBits = 4.85

// ParseParamsBillions extracts the parameter count (in billions) from a
// model reference such as "llama3.1:8b" or "org/Mixtral-8x7B-GGUF". MoE
// references count every expert, since all of them stay resident.
func ParseParamsBillions(ref string) (float64, bool) {
	m := paramsRe.FindStringSubmatch(ref)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[2], 64)
	if err != nil || n <= 0 {
		return 0, false
	}
	if m[1] != "" {
		experts, err := strconv.Atoi(m[1])
		if err == nil && experts > 0 {
			n *= float64(experts)
		}
	}
	return n, true
}

// weightBits returns the bits per weight for a model, and whether it came
// from an explicit quantization (as opposed to the engine's default).
func weightBits(engine, ref, quant string) (bits float64, explicit bool) {
	q := strings.ToUpper(strings.TrimSpace(quant))
	if q == "" {
		if m := refQuantRe.FindStringSubmatch(ref); m != nil {
			q = strings.TrimPrefix(strings.ToUpper(m[1]), "UD-")
		}
	}
	if b, ok := bitsPerWeight[q]; ok {
		return b, true
	}
	if engine == EngineVLLM {
		return 16, false
	}
	return defaultQuantBits, false
}

// EstimateWeightBytes sizes a model's weights from its name and
// quantization alone.
func EstimateWeightBytes(engine, ref, quant string) (int64, bool) {
	params, ok := ParseParamsBillions(ref)
	if !ok {
		return 0, false
	}
	bits, _ := weightBits(engine, ref, quant)
	return int64(params * 1e9 * bits / 8), true
}

func kvBytesPerToken(p FitParams, paramsB float64) float64 {
	if p.KVKiBPerToken > 0 {
		return p.KVKiBPerToken * 1024
	}
	kib := 320.0
	switch {
	case paramsB <= 2:
		kib = 48
	case paramsB <= 4:
		kib = 112
	case paramsB <= 9:
		kib = 128
	case paramsB <= 15:
		kib = 200
	case paramsB <= 40:
		kib = 256
	}
	return kib * 1024
}

// FitInput describes one model against one set of GPUs.
type FitInput struct {
	Engine        string
	ModelRef      string
	Quantization  string
	ContextLength int
	// WeightsBytes is the exact download size when known; 0 means estimate.
	WeightsBytes int64
	// GPUs is how many GPUs the model spreads over.
	GPUs int
	// FreeBytes is the total free VRAM across those GPUs; MinFreeBytes and
	// MinTotalBytes describe the tightest single GPU.
	FreeBytes     int64
	MinFreeBytes  int64
	MinTotalBytes int64
}

// FitDetail is the arithmetic behind a verdict.
type FitDetail struct {
	Verdict        string   `json:"verdict"`
	WeightsSource  string   `json:"weights_source"`
	WeightsBytes   int64    `json:"weights_bytes"`
	KVBytes        int64    `json:"kv_bytes"`
	OverheadBytes  int64    `json:"overhead_bytes"`
	NeedBytes      int64    `json:"need_bytes"`
	FreeBytes      int64    `json:"free_bytes"`
	ContextTokens  int      `json:"context_tokens"`
	ContextAssumed bool     `json:"context_assumed"`
	Arithmetic     string   `json:"arithmetic"`
	Reason         string   `json:"reason,omitempty"`
	Suggestions    []string `json:"suggestions"`
}

func formatGiB(b int64) string { return fmt.Sprintf("%.1f GiB", float64(b)/(1<<30)) }

// Check estimates whether in fits in its GPUs' free VRAM.
func (c FitConfig) Check(p FitParams, in FitInput) FitDetail {
	d := FitDetail{Verdict: FitUnknown, WeightsSource: WeightsUnknown, FreeBytes: in.FreeBytes, Suggestions: []string{}}
	d.ContextTokens = in.ContextLength
	if d.ContextTokens <= 0 {
		d.ContextTokens, d.ContextAssumed = p.DefaultContext, true
	}
	paramsB, haveParams := ParseParamsBillions(in.ModelRef)
	switch {
	case in.WeightsBytes > 0:
		d.WeightsBytes, d.WeightsSource = in.WeightsBytes, WeightsExact
	default:
		if w, ok := EstimateWeightBytes(in.Engine, in.ModelRef, in.Quantization); ok {
			d.WeightsBytes, d.WeightsSource = w, WeightsEstimated
		}
	}
	if d.WeightsBytes <= 0 {
		d.Reason = "The model size cannot be read from its name. Add a size such as 8b to the tag, or run a Hugging Face preflight for exact file sizes."
		d.Arithmetic = "model size unknown"
		return d
	}
	if in.FreeBytes < 0 || in.GPUs <= 0 {
		d.Reason = "This node has not reported its GPU memory yet."
		d.Arithmetic = "GPU memory unknown"
		return d
	}
	if haveParams {
		d.KVBytes = int64(kvBytesPerToken(p, paramsB) * float64(d.ContextTokens))
	} else {
		d.KVBytes = int64(0.15 * float64(d.WeightsBytes) * float64(d.ContextTokens) / 8192)
	}
	perGPU := p.GPUOverheadMiB
	if in.Engine == EngineVLLM {
		perGPU += p.VLLMExtraMiB
	}
	d.OverheadBytes = perGPU * mib * int64(in.GPUs)
	d.NeedBytes = d.WeightsBytes + d.KVBytes + d.OverheadBytes
	d.Arithmetic = fmt.Sprintf("weights %s + KV %s + overhead %s = %s of %s free", formatGiB(d.WeightsBytes), formatGiB(d.KVBytes), formatGiB(d.OverheadBytes), formatGiB(d.NeedBytes), formatGiB(in.FreeBytes))

	need, free := float64(d.NeedBytes), float64(in.FreeBytes)
	switch {
	case need <= free*c.FitPercent/100:
		d.Verdict = FitFits
	case need <= free:
		d.Verdict = FitTight
	default:
		d.Verdict = FitWontFit
	}
	if in.Engine == EngineVLLM && in.MinTotalBytes > 0 && float64(in.MinFreeBytes) < p.VLLMUtilization*float64(in.MinTotalBytes) {
		d.Verdict = FitWontFit
		d.Reason = fmt.Sprintf("vLLM reserves %.0f%% of each GPU at start and another workload holds VRAM on the tightest GPU (%s free of %s).",
			p.VLLMUtilization*100, formatGiB(in.MinFreeBytes), formatGiB(in.MinTotalBytes))
	}
	d.Suggestions = fitSuggestions(d, in)
	return d
}

func fitSuggestions(d FitDetail, in FitInput) []string {
	out := []string{}
	if d.Verdict != FitTight && d.Verdict != FitWontFit {
		return out
	}
	if !d.ContextAssumed && d.ContextTokens > 4096 {
		out = append(out, fmt.Sprintf("Lower the context length (KV cache is %s at %d tokens).", formatGiB(d.KVBytes), d.ContextTokens))
	} else if d.ContextAssumed {
		out = append(out, fmt.Sprintf("No context length is set, so %d tokens was assumed; set a smaller one to save KV cache.", d.ContextTokens))
	}
	if in.Engine != EngineVLLM {
		out = append(out, "Pick a smaller quantization such as Q4_K_M.")
	} else {
		out = append(out, "Use a quantized checkpoint (AWQ, GPTQ or FP8) or add GPUs for tensor parallelism.")
	}
	out = append(out, "Or place the model on a node with more free VRAM.")
	return out
}
