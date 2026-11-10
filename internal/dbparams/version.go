package dbparams

import (
	"sort"
	"strconv"
)

// minMajor gates parameters a later engine release introduced; a name absent
// here is valid on every version the allowlist covers.
var minMajor = map[Family]map[string]int{
	FamilyPostgres: {"jit": 11},
	FamilyRedis:    {"lazyfree-lazy-eviction": 4},
}

// leadingMajor reads the leading number of an image tag, false for tags such as "latest".
func leadingMajor(version string) (int, bool) {
	end := 0
	for end < len(version) && version[end] >= '0' && version[end] <= '9' {
		end++
	}
	n, err := strconv.Atoi(version[:end])
	return n, err == nil
}

// Incompatible returns the sorted names in params that the given engine
// version refuses, through the same validators staging uses plus the version gates.
func Incompatible(engine, version string, params map[string]string, lim Limits) []string {
	f, ok := FamilyFor(engine)
	var out []string
	for name, value := range params {
		if !ok {
			out = append(out, name)
			continue
		}
		if _, err := ValidateOne(engine, name, value, lim); err != nil {
			out = append(out, name)
			continue
		}
		if floor, gated := minMajor[f][name]; gated {
			if major, known := leadingMajor(version); known && major < floor {
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}
