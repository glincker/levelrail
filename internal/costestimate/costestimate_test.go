package costestimate

import (
	"testing"
	"time"
)

func TestEstimate(t *testing.T) {
	table := RateTable{
		Providers: []Provider{
			{Key: "vm", Label: "VM", CPUPerCoreUSD: 4, MemoryPerGiBUSD: 4, MinimumUSD: 4},
			{Key: "paas", Label: "PaaS", CPUPerCoreUSD: 25, MemoryPerGiBUSD: 10, MinimumUSD: 7},
		},
	}

	tests := []struct {
		name string
		in   Input
		want Result
	}{
		{
			name: "declared resources, above every minimum",
			in: Input{
				ServiceName:         "web",
				DeclaredNanoCPUs:    2_000_000_000, // 2 cores
				DeclaredMemoryBytes: 4 * gib,       // 4 GiB
			},
			want: Result{
				ServiceName: "web",
				VCPUCores:   2,
				MemoryGiB:   4,
				CPUBasis:    BasisDeclared,
				MemoryBasis: BasisDeclared,
				Providers: []ProviderEstimate{
					{Key: "vm", Label: "VM", CPUCostUSD: 8, MemoryCostUSD: 16, TotalUSD: 24},
					{Key: "paas", Label: "PaaS", CPUCostUSD: 50, MemoryCostUSD: 40, TotalUSD: 90},
				},
			},
		},
		{
			name: "tiny declared allocation hits the provider minimum",
			in: Input{
				ServiceName:         "sidecar",
				DeclaredNanoCPUs:    100_000_000, // 0.1 core
				DeclaredMemoryBytes: 64 * 1024 * 1024,
			},
			want: Result{
				ServiceName: "sidecar",
				VCPUCores:   0.1,
				MemoryGiB:   64.0 / 1024,
				CPUBasis:    BasisDeclared,
				MemoryBasis: BasisDeclared,
				Providers: []ProviderEstimate{
					{Key: "vm", Label: "VM", CPUCostUSD: 0.4, MemoryCostUSD: 0.25, TotalUSD: 4, MinimumApplied: true},
					{Key: "paas", Label: "PaaS", CPUCostUSD: 2.5, MemoryCostUSD: 0.63, TotalUSD: 7, MinimumApplied: true},
				},
			},
		},
		{
			name: "no declared limits, falls back to observed p95 usage",
			in: Input{
				ServiceName: "api",
				ObservedCPUPercentSamples: []Sample{
					{Timestamp: time.Unix(0, 0), Value: 50},
					{Timestamp: time.Unix(1, 0), Value: 100},
					{Timestamp: time.Unix(2, 0), Value: 150},
					{Timestamp: time.Unix(3, 0), Value: 200},
				},
				ObservedMemoryBytesSamples: []Sample{
					{Timestamp: time.Unix(0, 0), Value: 1 * gib},
					{Timestamp: time.Unix(1, 0), Value: 2 * gib},
					{Timestamp: time.Unix(2, 0), Value: 2 * gib},
				},
			},
			want: Result{
				ServiceName: "api",
				VCPUCores:   2,
				MemoryGiB:   2,
				CPUBasis:    BasisObserved,
				MemoryBasis: BasisObserved,
				Providers: []ProviderEstimate{
					{Key: "vm", Label: "VM", CPUCostUSD: 8, MemoryCostUSD: 8, TotalUSD: 16},
					{Key: "paas", Label: "PaaS", CPUCostUSD: 50, MemoryCostUSD: 20, TotalUSD: 70},
				},
			},
		},
		{
			name: "too few observed samples and no declared limit is unavailable, not a fabricated number",
			in: Input{
				ServiceName: "fresh",
				ObservedCPUPercentSamples: []Sample{
					{Timestamp: time.Unix(0, 0), Value: 100},
				},
			},
			want: Result{
				ServiceName: "fresh",
				VCPUCores:   0,
				MemoryGiB:   0,
				CPUBasis:    BasisUnavailable,
				MemoryBasis: BasisUnavailable,
				Providers: []ProviderEstimate{
					{Key: "vm", Label: "VM", CPUCostUSD: 0, MemoryCostUSD: 0, TotalUSD: 4, MinimumApplied: true},
					{Key: "paas", Label: "PaaS", CPUCostUSD: 0, MemoryCostUSD: 0, TotalUSD: 7, MinimumApplied: true},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Estimate(tc.in, table)
			if got.ServiceName != tc.want.ServiceName ||
				!floatEqual(got.VCPUCores, tc.want.VCPUCores) ||
				!floatEqual(got.MemoryGiB, tc.want.MemoryGiB) ||
				got.CPUBasis != tc.want.CPUBasis ||
				got.MemoryBasis != tc.want.MemoryBasis {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			if len(got.Providers) != len(tc.want.Providers) {
				t.Fatalf("got %d providers, want %d", len(got.Providers), len(tc.want.Providers))
			}
			for i, wp := range tc.want.Providers {
				gp := got.Providers[i]
				if gp.Key != wp.Key || gp.Label != wp.Label ||
					!floatEqual(gp.CPUCostUSD, wp.CPUCostUSD) ||
					!floatEqual(gp.MemoryCostUSD, wp.MemoryCostUSD) ||
					!floatEqual(gp.TotalUSD, wp.TotalUSD) ||
					gp.MinimumApplied != wp.MinimumApplied {
					t.Fatalf("provider %d: got %+v, want %+v", i, gp, wp)
				}
			}
		})
	}
}

func floatEqual(a, b float64) bool {
	const epsilon = 1e-9
	diff := a - b
	if diff < 0 {
		diff = -diff
	}
	return diff < epsilon
}

func TestRateTableFromEnv(t *testing.T) {
	tests := []struct {
		name   string
		lookup map[string]string
		check  func(t *testing.T, table RateTable)
	}{
		{
			name:   "no overrides keeps defaults",
			lookup: map[string]string{},
			check: func(t *testing.T, table RateTable) {
				def := DefaultRateTable()
				if table.Providers[0].CPUPerCoreUSD != def.Providers[0].CPUPerCoreUSD {
					t.Fatalf("expected default CPU rate, got %v", table.Providers[0].CPUPerCoreUSD)
				}
			},
		},
		{
			name: "valid override replaces the rate",
			lookup: map[string]string{
				"APP_COST_ESTIMATE_VM_CPU_USD": "9.5",
			},
			check: func(t *testing.T, table RateTable) {
				if table.Providers[0].Key != ProviderVM {
					t.Fatalf("expected first provider to be vm, got %q", table.Providers[0].Key)
				}
				if table.Providers[0].CPUPerCoreUSD != 9.5 {
					t.Fatalf("expected overridden CPU rate 9.5, got %v", table.Providers[0].CPUPerCoreUSD)
				}
			},
		},
		{
			name: "malformed override is ignored",
			lookup: map[string]string{
				"APP_COST_ESTIMATE_VM_CPU_USD": "not-a-number",
			},
			check: func(t *testing.T, table RateTable) {
				def := DefaultRateTable()
				if table.Providers[0].CPUPerCoreUSD != def.Providers[0].CPUPerCoreUSD {
					t.Fatalf("expected default to survive malformed override, got %v", table.Providers[0].CPUPerCoreUSD)
				}
			},
		},
		{
			name: "negative override is ignored",
			lookup: map[string]string{
				"APP_COST_ESTIMATE_VM_MEMORY_USD": "-1",
			},
			check: func(t *testing.T, table RateTable) {
				def := DefaultRateTable()
				if table.Providers[0].MemoryPerGiBUSD != def.Providers[0].MemoryPerGiBUSD {
					t.Fatalf("expected default to survive negative override, got %v", table.Providers[0].MemoryPerGiBUSD)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(key string) (string, bool) {
				v, ok := tc.lookup[key]
				return v, ok
			}
			table := rateTableFromLookup(lookup)
			tc.check(t, table)
		})
	}
}
