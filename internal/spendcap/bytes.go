package spendcap

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	kb = int64(1000)
	mb = kb * 1000
	gb = mb * 1000
	tb = gb * 1000
)

var byteUnits = map[string]int64{
	"": 1, "b": 1,
	"k": kb, "kb": kb, "m": mb, "mb": mb, "g": gb, "gb": gb, "t": tb, "tb": tb,
	"ki": 1 << 10, "kib": 1 << 10, "mi": 1 << 20, "mib": 1 << 20, "gi": 1 << 30, "gib": 1 << 30, "ti": 1 << 40, "tib": 1 << 40,
}

// ParseBytes reads 500GiB (binary) or 500GB (decimal) into bytes.
func ParseBytes(s string) (int64, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	i := 0
	for i < len(t) && (t[i] >= '0' && t[i] <= '9' || t[i] == '.') {
		i++
	}
	mult, ok := byteUnits[strings.TrimSpace(t[i:])]
	n, err := strconv.ParseFloat(t[:i], 64)
	if !ok || err != nil || n < 0 || n*float64(mult) > float64(maxEgressBytes) {
		return 0, fmt.Errorf("invalid size %q, want a number with a unit such as 500GiB or 2TB", s)
	}
	return int64(n * float64(mult)), nil
}
