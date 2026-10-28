package k8simport

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

var quantitySuffix = []struct {
	s string
	m float64
}{
	{"Ki", 1 << 10}, {"Mi", 1 << 20}, {"Gi", 1 << 30}, {"Ti", 1 << 40}, {"Pi", 1 << 50}, {"Ei", 1 << 60},
	{"n", 1e-9}, {"u", 1e-6}, {"m", 1e-3},
	{"k", 1e3}, {"K", 1e3}, {"M", 1e6}, {"G", 1e9}, {"T", 1e12}, {"P", 1e15}, {"E", 1e18},
}

// parseQuantity returns the value of a Kubernetes quantity in base units.
func parseQuantity(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty quantity")
	}
	mult := 1.0
	for _, suf := range quantitySuffix {
		if strings.HasSuffix(s, suf.s) {
			s, mult = strings.TrimSuffix(s, suf.s), suf.m
			break
		}
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, fmt.Errorf("not a quantity")
	}
	return v * mult, nil
}

// memoryString renders bytes as the NMi or NGi form the spec uses.
func memoryString(b float64) string {
	const mi = 1 << 20
	mib := int64(math.Ceil(b / mi))
	if mib < 1 {
		mib = 1
	}
	if mib%1024 == 0 {
		return strconv.FormatInt(mib/1024, 10) + "Gi"
	}
	return strconv.FormatInt(mib, 10) + "Mi"
}

func cpuValue(v float64) float64 { return math.Round(v*1000) / 1000 }
