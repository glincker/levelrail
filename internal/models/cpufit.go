package models

import (
	"fmt"
	"strings"
)

// CPU fit verdicts.
const (
	CPUFitComfortable = "fits_comfortably"
	CPUFitTight       = "tight"
	CPUFitWontFit     = "wont_fit"
	CPUFitUnknown     = "unknown"
)

// CPUFitNote states what the CPU preflight can and cannot promise.
const CPUFitNote = "Memory and speed are estimates, not measurements: weights come from the file size, the KV cache is approximated from the " +
	"parameter count and context, and speed assumes memory bandwidth scales with the CPUs you allow. Run the benchmark on the running model for real numbers."

// CPUFitInput describes a CPU model against a node and its caps.
type CPUFitInput struct {
	ModelRef      string
	Quantization  string
	ContextTokens int
	// WeightsBytes is the exact file size, 0 to estimate from the name.
	WeightsBytes int64
	// ParamsB overrides the count parsed from the name when positive.
	ParamsB float64
	// EntryID is the catalog id, used to avoid suggesting the same model.
	EntryID string
	Host    HostFacts
	// CPUMilli and MemoryMiB are the requested caps, 0 for defaults.
	CPUMilli  int
	MemoryMiB int
	// DiskFreeBytes is -1 when unknown.
	DiskFreeBytes int64
}

// CPUFit is the preflight answer for CPU mode.
type CPUFit struct {
	Verdict        string   `json:"verdict"`
	Reason         string   `json:"reason"`
	Suggestion     string   `json:"suggestion,omitempty"`
	SuggestedID    string   `json:"suggested_id,omitempty"`
	CPUMilli       int      `json:"cpu_milli"`
	MemoryMiB      int      `json:"memory_mib"`
	ContextTokens  int      `json:"context_tokens"`
	WeightsBytes   int64    `json:"weights_bytes"`
	WeightsSource  string   `json:"weights_source"`
	KVBytes        int64    `json:"kv_bytes"`
	OverheadBytes  int64    `json:"overhead_bytes"`
	NeedBytes      int64    `json:"need_bytes"`
	CapBytes       int64    `json:"cap_bytes"`
	DiskBytes      int64    `json:"disk_bytes"`
	DiskOK         *bool    `json:"disk_ok"`
	TokensPerSecLo float64  `json:"tokens_per_sec_low"`
	TokensPerSecHi float64  `json:"tokens_per_sec_high"`
	Estimate       bool     `json:"estimate"`
	Arithmetic     string   `json:"arithmetic"`
	Warnings       []string `json:"warnings"`
	Note           string   `json:"note"`
}

// Check rates a CPU model. It never guesses a size it cannot read.
func (p CPUPolicy) Check(cat Catalog, fp FitParams, in CPUFitInput) CPUFit {
	out := CPUFit{Verdict: CPUFitUnknown, WeightsSource: WeightsUnknown, Estimate: true, Warnings: []string{}, Note: CPUFitNote}
	lim, err := p.ResolveCPULimits(in.Host, in.CPUMilli, in.MemoryMiB, in.ContextTokens)
	if err != nil {
		out.Verdict, out.Reason = CPUFitWontFit, strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": ")
		out.ContextTokens = in.ContextTokens
		return out
	}
	out.CPUMilli, out.MemoryMiB, out.ContextTokens = lim.CPUMilli, lim.MemoryMiB, lim.Context
	out.CapBytes = int64(lim.MemoryMiB) * mib

	paramsB, haveParams := in.ParamsB, in.ParamsB > 0
	if !haveParams {
		paramsB, haveParams = ParseParamsBillions(in.ModelRef)
	}
	switch {
	case in.WeightsBytes > 0:
		out.WeightsBytes, out.WeightsSource = in.WeightsBytes, WeightsExact
	default:
		if w, ok := EstimateWeightBytes(EngineLlamaCpp, in.ModelRef, in.Quantization); ok {
			out.WeightsBytes, out.WeightsSource = w, WeightsEstimated
		}
	}
	if out.WeightsBytes <= 0 {
		out.Reason = "The model size cannot be read from its name. Pick a curated model, add a size such as 3b to the name, or run a Hugging Face preflight for exact file sizes."
		out.Arithmetic = "model size unknown"
		return out
	}
	if haveParams {
		out.KVBytes = int64(kvBytesPerToken(fp, paramsB) * float64(lim.Context))
	} else {
		out.KVBytes = int64(0.15 * float64(out.WeightsBytes) * float64(lim.Context) / 8192)
	}
	out.OverheadBytes = (p.OverheadMiB + int64(p.CacheMiB(lim.MemoryMiB))) * mib
	out.NeedBytes = out.WeightsBytes + out.KVBytes + out.OverheadBytes
	out.DiskBytes = out.WeightsBytes
	out.Arithmetic = fmt.Sprintf("weights %s + KV %s + overhead %s = %s of a %s cap", formatGiB(out.WeightsBytes), formatGiB(out.KVBytes),
		formatGiB(out.OverheadBytes), formatGiB(out.NeedBytes), formatGiB(out.CapBytes))

	cpus := float64(lim.CPUMilli) / 1000
	bw := min(cpus*p.BandwidthPerCPUGBs, p.BandwidthMaxGBs)
	ceil := bw / (float64(out.WeightsBytes) / 1e9)
	out.TokensPerSecHi, out.TokensPerSecLo = round1(ceil*0.7), round1(ceil*0.25)

	need, capB := float64(out.NeedBytes), float64(out.CapBytes)
	switch {
	case need <= capB*p.ComfortPercent/100:
		out.Verdict = CPUFitComfortable
		out.Reason = "The weights, context and engine overhead fit well inside the memory cap."
	case need <= capB:
		out.Verdict = CPUFitTight
		out.Reason = "It fits the memory cap with little to spare; a long prompt or a busy server may push it over."
	default:
		out.Verdict = CPUFitWontFit
		out.Reason = fmt.Sprintf("Weights plus KV cache plus overhead need %s but the cap is %s.", formatGiB(out.NeedBytes), formatGiB(out.CapBytes))
	}
	if in.Host.MemAvailBytes > 0 && in.Host.MemAvailBytes-out.CapBytes < p.FreeRAMFloorMiB*mib {
		out.Warnings = append(out.Warnings, fmt.Sprintf("Only %s of RAM is free right now; taking the full cap would leave under %d MiB for other workloads.",
			formatGiB(in.Host.MemAvailBytes), p.FreeRAMFloorMiB))
		if out.Verdict == CPUFitComfortable {
			out.Verdict = CPUFitTight
			out.Reason = "It fits the cap, but the server has little free RAM right now."
		}
	}
	if in.DiskFreeBytes >= 0 {
		ok := in.DiskFreeBytes >= out.DiskBytes+out.DiskBytes/10
		out.DiskOK = &ok
		if !ok {
			out.Verdict = CPUFitWontFit
			out.Reason = fmt.Sprintf("The download needs about %s of disk and only %s is free.", formatGiB(out.DiskBytes), formatGiB(in.DiskFreeBytes))
		}
	}
	if out.Verdict == CPUFitTight || out.Verdict == CPUFitWontFit {
		out.Suggestion, out.SuggestedID = suggestSmaller(cat, p, fp, in, lim, out)
	}
	return out
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

// suggestSmaller names a smaller context, or the largest curated chat
// model that is smaller than this one and fits the cap.
func suggestSmaller(cat Catalog, p CPUPolicy, fp FitParams, in CPUFitInput, lim CPULimits, cur CPUFit) (text, id string) {
	if lim.Context > 2048 && cur.NeedBytes-cur.KVBytes/2 <= cur.CapBytes {
		text = fmt.Sprintf("Lower the context to %d tokens to save %s of KV cache.", lim.Context/2, formatGiB(cur.KVBytes/2))
	}
	var best *CatalogEntry
	sorted := cat.Sorted()
	for i := range sorted {
		e := sorted[i]
		if e.Kind != KindChat || e.ID == in.EntryID || e.DownloadBytes >= cur.WeightsBytes {
			continue
		}
		need := e.DownloadBytes + int64(kvBytesPerToken(fp, e.ParamsB)*float64(lim.Context)) + cur.OverheadBytes
		if float64(need) <= float64(cur.CapBytes)*p.ComfortPercent/100 && (best == nil || e.DownloadBytes > best.DownloadBytes) {
			best = &sorted[i]
		}
	}
	if best != nil {
		msg := fmt.Sprintf("Try %s (%s download), which fits this cap.", best.Label, formatGiB(best.DownloadBytes))
		if text != "" {
			msg = text + " " + msg
		}
		return msg, best.ID
	}
	if text == "" {
		text = "Raise the memory cap or choose a smaller model."
	}
	return text, ""
}
