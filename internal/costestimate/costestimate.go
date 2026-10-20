// Package costestimate is a deterministic, pure-Go engine that turns an
// app's CPU/memory allocation into an ESTIMATED monthly cost under a
// few generic reference pricing profiles (a plain VM, a managed VM, a
// metered container PaaS). Not a real bill: it never calls a pricing
// API or reflects actual hardware cost, only a comparative "what would
// this cost under typical metered cloud pricing" number.
package costestimate

import (
	"math"
	"os"
	"sort"
	"strconv"
	"time"
)

// Sample is one raw (timestamp, value) usage reading, the same shape
// internal/rightsizing.Sample already establishes for this purpose.
type Sample struct {
	Timestamp time.Time
	Value     float64
}

// Basis records whether a dimension's size came from a configured
// resource limit or was inferred from observed usage history, so the
// caller can say which one a given estimate rests on.
const (
	BasisDeclared    = "declared"
	BasisObserved    = "observed"
	BasisUnavailable = "unavailable"
)

// Provider is one reference pricing profile: a flat $/vCPU-core/month
// and $/GiB-month rate, plus a minimum monthly charge (most real
// providers bill a floor even for a tiny instance). Key is the stable
// identifier used in JSON and in APP_COST_ESTIMATE_<KEY>_* env
// overrides; Label is what a human reads.
type Provider struct {
	Key             string
	Label           string
	CPUPerCoreUSD   float64
	MemoryPerGiBUSD float64
	MinimumUSD      float64
}

// RateTable is the full set of reference providers an estimate is
// computed under. Every number here is illustrative, not fetched from
// any live pricing source, and is meant to be corrected per-operator
// via env vars for their own region/provider.
type RateTable struct {
	Providers []Provider
}

// Provider keys, also the env var infix (uppercased) for overrides.
const (
	ProviderVM          = "vm"
	ProviderManagedVM   = "managed_vm"
	ProviderMeteredPaaS = "paas"
)

// DefaultRateTable returns the built-in reference rates: a bare
// unmanaged cloud VM, a managed VM (backups/support bundled in), and a
// metered container PaaS (Heroku/Render/Railway-shaped pricing, which
// bundles a real margin over raw compute). These are deliberately
// round, approximate numbers for 2026-era budget cloud pricing, not any
// single real provider's rate card. Override any of them with
// APP_COST_ESTIMATE_<KEY>_CPU_USD, APP_COST_ESTIMATE_<KEY>_MEMORY_USD,
// APP_COST_ESTIMATE_<KEY>_MINIMUM_USD.
func DefaultRateTable() RateTable {
	return RateTable{
		Providers: []Provider{
			{
				Key:             ProviderVM,
				Label:           "Generic cloud VM",
				CPUPerCoreUSD:   4.0,
				MemoryPerGiBUSD: 4.0,
				MinimumUSD:      4.0,
			},
			{
				Key:             ProviderManagedVM,
				Label:           "Generic managed VM",
				CPUPerCoreUSD:   8.0,
				MemoryPerGiBUSD: 6.0,
				MinimumUSD:      10.0,
			},
			{
				Key:             ProviderMeteredPaaS,
				Label:           "Generic metered container PaaS",
				CPUPerCoreUSD:   25.0,
				MemoryPerGiBUSD: 10.0,
				MinimumUSD:      7.0,
			},
		},
	}
}

// envPrefix namespaces every override this package reads, matching the
// APP_<THING>_* convention internal/alerting's SLOPolicyFromEnv and this
// project's APP_BRAND_*/APP_DATA_DIR already establish.
const envPrefix = "APP_COST_ESTIMATE_"

// RateTableFromEnv applies APP_COST_ESTIMATE_<KEY>_CPU_USD,
// APP_COST_ESTIMATE_<KEY>_MEMORY_USD and
// APP_COST_ESTIMATE_<KEY>_MINIMUM_USD overrides to DefaultRateTable's
// providers (KEY is the provider's own Key, uppercased: VM, MANAGED_VM,
// PAAS). A malformed or negative value keeps the default, matching
// SLOPolicyFromEnv's own "bad input is ignored, not fatal" contract.
func RateTableFromEnv() RateTable {
	return rateTableFromLookup(os.LookupEnv)
}

func rateTableFromLookup(lookup func(string) (string, bool)) RateTable {
	table := DefaultRateTable()
	for i := range table.Providers {
		p := &table.Providers[i]
		key := upperSnake(p.Key)
		applyFloatOverride(lookup, envPrefix+key+"_CPU_USD", &p.CPUPerCoreUSD)
		applyFloatOverride(lookup, envPrefix+key+"_MEMORY_USD", &p.MemoryPerGiBUSD)
		applyFloatOverride(lookup, envPrefix+key+"_MINIMUM_USD", &p.MinimumUSD)
	}
	return table
}

func applyFloatOverride(lookup func(string) (string, bool), name string, dst *float64) {
	raw, ok := lookup(name)
	if !ok {
		return
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 {
		return
	}
	*dst = v
}

func upperSnake(key string) string {
	out := make([]byte, 0, len(key))
	for _, c := range []byte(key) {
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		out = append(out, c)
	}
	return string(out)
}

const (
	gib = 1024 * 1024 * 1024

	// minSampleCount gates whether observed usage is trusted as a
	// fallback basis: a single stray sample should not anchor a cost
	// estimate, the same spirit as internal/rightsizing's own
	// minSampleCount gate but with a lower bar since this only needs a
	// size, not a confident recommendation.
	minSampleCount = 3
)

// Input bundles one app's declared resource limits and, when a limit
// isn't configured, the observed usage samples to fall back to.
// CPUPercentSamples follows Docker's own 0-100-per-core convention
// (matching internal/rightsizing.Input's own doc comment): a container
// fully using 2 cores reads 200.
type Input struct {
	ServiceName                string
	DeclaredNanoCPUs           int64
	DeclaredMemoryBytes        int64
	ObservedCPUPercentSamples  []Sample
	ObservedMemoryBytesSamples []Sample
}

// ProviderEstimate is one reference provider's resulting monthly cost,
// split into its CPU and memory components so the UI can show a
// breakdown rather than just a total.
type ProviderEstimate struct {
	Key            string
	Label          string
	CPUCostUSD     float64
	MemoryCostUSD  float64
	TotalUSD       float64
	MinimumApplied bool
}

// Result is Estimate's output: the resolved CPU/memory size this
// estimate is based on, which basis (declared vs observed) produced
// each dimension, and one ProviderEstimate per reference provider in
// table, in table order.
type Result struct {
	ServiceName string
	VCPUCores   float64
	MemoryGiB   float64
	CPUBasis    string
	MemoryBasis string
	Providers   []ProviderEstimate
}

// Estimate computes in's estimated monthly cost under every provider in
// table. The same Input and RateTable always produce the same Result.
// This is an ESTIMATE only: it reflects the declared or observed
// resource envelope under illustrative reference rates, never a real
// invoice from any provider.
func Estimate(in Input, table RateTable) Result {
	vcpu, cpuBasis := resolveCPU(in)
	memGiB, memBasis := resolveMemory(in)

	providers := make([]ProviderEstimate, 0, len(table.Providers))
	for _, p := range table.Providers {
		cpuCost := vcpu * p.CPUPerCoreUSD
		memCost := memGiB * p.MemoryPerGiBUSD
		total := cpuCost + memCost
		minimumApplied := false
		if total < p.MinimumUSD {
			total = p.MinimumUSD
			minimumApplied = true
		}
		providers = append(providers, ProviderEstimate{
			Key:            p.Key,
			Label:          p.Label,
			CPUCostUSD:     roundCents(cpuCost),
			MemoryCostUSD:  roundCents(memCost),
			TotalUSD:       roundCents(total),
			MinimumApplied: minimumApplied,
		})
	}

	return Result{
		ServiceName: in.ServiceName,
		VCPUCores:   vcpu,
		MemoryGiB:   memGiB,
		CPUBasis:    cpuBasis,
		MemoryBasis: memBasis,
		Providers:   providers,
	}
}

func resolveCPU(in Input) (float64, string) {
	if in.DeclaredNanoCPUs > 0 {
		return float64(in.DeclaredNanoCPUs) / 1_000_000_000, BasisDeclared
	}
	if len(in.ObservedCPUPercentSamples) >= minSampleCount {
		values := make([]float64, len(in.ObservedCPUPercentSamples))
		for i, s := range in.ObservedCPUPercentSamples {
			values[i] = s.Value
		}
		return percentile(values, 0.95) / 100, BasisObserved
	}
	return 0, BasisUnavailable
}

func resolveMemory(in Input) (float64, string) {
	if in.DeclaredMemoryBytes > 0 {
		return float64(in.DeclaredMemoryBytes) / gib, BasisDeclared
	}
	if len(in.ObservedMemoryBytesSamples) >= minSampleCount {
		values := make([]float64, len(in.ObservedMemoryBytesSamples))
		for i, s := range in.ObservedMemoryBytesSamples {
			values[i] = s.Value
		}
		return percentile(values, 0.95) / gib, BasisObserved
	}
	return 0, BasisUnavailable
}

// percentile uses the nearest-rank method over a defensive copy of
// values, sorted ascending; p is a fraction in [0, 1]. Matches
// internal/rightsizing's own percentile helper, kept local so this
// package doesn't depend on that one for a three-line function.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func roundCents(v float64) float64 {
	return math.Round(v*100) / 100
}
