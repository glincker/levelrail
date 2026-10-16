package models

import (
	"strings"
	"testing"
)

func TestCPUFitVerdicts(t *testing.T) {
	cat, err := LocalCatalog()
	if err != nil {
		t.Fatal(err)
	}
	host8 := HostFacts{CPUs: 4, MemTotalBytes: 8 * gib, MemAvailBytes: 6 * gib}
	tests := []struct {
		name    string
		in      CPUFitInput
		want    string
		reason  string
		suggest string
	}{
		{"3b fits comfortably in 4 GiB", CPUFitInput{ModelRef: "ibm-granite/granite-4.1-3b-GGUF:Q4_K_M", WeightsBytes: 2099501664, ParamsB: 3.4,
			Host: host8, MemoryMiB: 4096, DiskFreeBytes: 50 * gib}, CPUFitComfortable, "fit", ""},
		{"3b is tight in 3 GiB", CPUFitInput{ModelRef: "ibm-granite/granite-4.1-3b-GGUF:Q4_K_M", WeightsBytes: 2099501664, ParamsB: 3.4,
			Host: host8, MemoryMiB: 3300, DiskFreeBytes: 50 * gib}, CPUFitTight, "little to spare", ""},
		{"gemma will not fit in 2 GiB and a smaller model is suggested", CPUFitInput{ModelRef: "unsloth/gemma-4-E4B-it-GGUF:Q4_K_M", WeightsBytes: 4977171584, ParamsB: 7.5,
			EntryID: "gemma-4-e4b", Host: host8, MemoryMiB: 3072, DiskFreeBytes: 50 * gib}, CPUFitWontFit, "need", "Try"},
		{"disk too small refuses", CPUFitInput{ModelRef: "ibm-granite/granite-4.1-3b-GGUF:Q4_K_M", WeightsBytes: 2099501664, ParamsB: 3.4,
			Host: host8, MemoryMiB: 4096, DiskFreeBytes: 1 * gib}, CPUFitWontFit, "disk", ""},
		{"unknown size is unknown, never guessed", CPUFitInput{ModelRef: "someone/mystery-GGUF", Host: host8, DiskFreeBytes: 50 * gib}, CPUFitUnknown, "cannot be read", ""},
		{"size from the name is estimated", CPUFitInput{ModelRef: "org/Foo-3B-GGUF:Q4_K_M", Host: host8, MemoryMiB: 4096, DiskFreeBytes: -1}, CPUFitComfortable, "", ""},
		{"cap beyond the node share is refused", CPUFitInput{ModelRef: "ibm-granite/granite-4.0-1b-GGUF:Q4_K_M", WeightsBytes: 1023645440, ParamsB: 1.6,
			Host: host8, MemoryMiB: 7000}, CPUFitWontFit, "above", ""},
		{"low free RAM warns and downgrades", CPUFitInput{ModelRef: "ibm-granite/granite-4.1-3b-GGUF:Q4_K_M", WeightsBytes: 2099501664, ParamsB: 3.4,
			Host: HostFacts{CPUs: 4, MemTotalBytes: 8 * gib, MemAvailBytes: 4500 << 20}, MemoryMiB: 4096, DiskFreeBytes: 50 * gib}, CPUFitTight, "free RAM", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := testPolicy().Check(cat, FitParams{DefaultContext: 8192}, tt.in)
			if got.Verdict != tt.want {
				t.Fatalf("verdict = %q (%s), want %q", got.Verdict, got.Reason, tt.want)
			}
			if !strings.Contains(strings.ToLower(got.Reason+" "+strings.Join(got.Warnings, " ")), strings.ToLower(tt.reason)) {
				t.Errorf("reason %q / warnings %v lack %q", got.Reason, got.Warnings, tt.reason)
			}
			if tt.suggest != "" && !strings.Contains(got.Suggestion, tt.suggest) {
				t.Errorf("suggestion = %q, want %q", got.Suggestion, tt.suggest)
			}
			if !got.Estimate {
				t.Error("results must always be labelled as estimates")
			}
			if got.Verdict != CPUFitUnknown && got.Verdict != CPUFitWontFit || got.NeedBytes > 0 {
				if got.TokensPerSecHi < got.TokensPerSecLo {
					t.Errorf("tps range inverted: %v..%v", got.TokensPerSecLo, got.TokensPerSecHi)
				}
			}
		})
	}
}

func TestCPUFitSpeedScalesWithCPUs(t *testing.T) {
	cat, _ := LocalCatalog()
	run := func(milli int) CPUFit {
		return testPolicy().Check(cat, FitParams{DefaultContext: 8192}, CPUFitInput{ModelRef: "ibm-granite/granite-4.1-3b-GGUF:Q4_K_M", WeightsBytes: 2099501664,
			ParamsB: 3.4, Host: HostFacts{CPUs: 16, MemTotalBytes: 32 * gib}, CPUMilli: milli, MemoryMiB: 4096, DiskFreeBytes: -1})
	}
	if lo, hi := run(1000), run(4000); !(hi.TokensPerSecHi > lo.TokensPerSecHi) {
		t.Errorf("4 cpus (%v) should estimate faster than 1 (%v)", hi.TokensPerSecHi, lo.TokensPerSecHi)
	}
}
