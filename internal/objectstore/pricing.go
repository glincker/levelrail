package objectstore

import (
	"strconv"
	"strings"
)

// PricePrefix is the env prefix for overriding a preset's USD per GB-month, for example APP_STORAGE_PRICE_PER_GB_MONTH_R2.
const PricePrefix = "APP_STORAGE_PRICE_PER_GB_MONTH_"

var defaultPricePerGBMonth = map[string]float64{
	PresetAWS: 0.023, PresetR2: 0.015, PresetB2: 0.006, PresetWasabi: 0.0069, PresetGCS: 0.020,
}

// PricePerGBMonth is the list price used for cost estimates; self-hosted and node directories cost nothing.
func PricePerGBMonth(preset string, lookup func(string) (string, bool)) float64 {
	if lookup != nil {
		if v, ok := lookup(PricePrefix + strings.ToUpper(preset)); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f >= 0 {
				return f
			}
		}
	}
	return defaultPricePerGBMonth[preset]
}
