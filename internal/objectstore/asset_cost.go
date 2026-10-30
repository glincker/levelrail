package objectstore

import (
	"os"
	"strings"
)

// PriceTableDate is when the default list prices in pricing.go were last checked against provider price pages.
const PriceTableDate = "2026-10-25"

const (
	bytesPerGB = 1e9
	// B2FreeEgressMultiple is Backblaze B2's free monthly egress as a multiple of the stored size.
	B2FreeEgressMultiple = 3.0
)

// CostLine prices one preset for the given stored size and monthly egress.
type CostLine struct {
	Preset       string  `json:"preset"`
	StorageUSD   float64 `json:"storage_usd"`
	EgressUSD    float64 `json:"egress_usd"`
	TotalUSD     float64 `json:"total_usd"`
	FreeEgressGB float64 `json:"free_egress_gb"`
}

// CostEstimate is always an estimate: list prices, no request charges, no tiers beyond B2's free egress.
type CostEstimate struct {
	Estimated      bool       `json:"estimated"`
	PriceTableDate string     `json:"price_table_date"`
	StoredBytes    int64      `json:"stored_bytes"`
	EgressGB       float64    `json:"egress_gb"`
	Current        CostLine   `json:"current"`
	Compare        []CostLine `json:"compare"`
	OverrideEnvs   []string   `json:"override_envs"`
	Notes          []string   `json:"notes"`
}

// CostLineFor prices one preset. lookup resolves the price overrides (os.LookupEnv in production).
func CostLineFor(preset string, storedBytes int64, egressGB float64, lookup func(string) (string, bool)) CostLine {
	storedGB := float64(storedBytes) / bytesPerGB
	line := CostLine{Preset: preset, StorageUSD: storedGB * PricePerGBMonth(preset, lookup)}
	if preset == PresetB2 {
		line.FreeEgressGB = storedGB * B2FreeEgressMultiple
	}
	billable := max(egressGB-line.FreeEgressGB, 0)
	line.EgressUSD = billable * EgressPricePerGB(preset, lookup)
	line.TotalUSD = line.StorageUSD + line.EgressUSD
	return line
}

// EstimateAssetCost prices the bucket's preset and the same volume on S3 and R2 for comparison.
func EstimateAssetCost(preset string, storedBytes int64, egressGB float64, lookup func(string) (string, bool)) CostEstimate {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	up := strings.ToUpper(preset)
	return CostEstimate{
		Estimated: true, PriceTableDate: PriceTableDate, StoredBytes: storedBytes, EgressGB: egressGB,
		Current: CostLineFor(preset, storedBytes, egressGB, lookup),
		Compare: []CostLine{
			CostLineFor(PresetAWS, storedBytes, egressGB, lookup),
			CostLineFor(PresetR2, storedBytes, egressGB, lookup),
		},
		OverrideEnvs: []string{PricePrefix + up, EgressPricePrefix + up},
		Notes: []string{
			"Estimated from list prices, not your invoice.",
			"Request (operation) charges and free tiers other than Backblaze B2's free egress are not modeled.",
			"Egress is the monthly volume you entered; Direct serving mode does not measure traffic.",
		},
	}
}
