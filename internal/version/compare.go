package version

import (
	"strconv"
	"strings"
)

// Compare orders two release versions such as "v1.2.3" or "0.4.0-beta.2"
// by semver precedence: -1 if a < b, 0 if equal, 1 if a > b. ok is false
// when either is not a release version (for example "dev").
func Compare(a, b string) (cmp int, ok bool) {
	pa, okA := parse(a)
	pb, okB := parse(b)
	if !okA || !okB {
		return 0, false
	}
	for i := range pa.core {
		if c := compareInt(pa.core[i], pb.core[i]); c != 0 {
			return c, true
		}
	}
	return comparePre(pa.pre, pb.pre), true
}

type parsed struct {
	core [3]int
	pre  []string
}

func parse(v string) (parsed, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	var p parsed
	core := v
	if i := strings.IndexByte(v, '-'); i >= 0 {
		core = v[:i]
		p.pre = strings.Split(v[i+1:], ".")
	}
	parts := strings.Split(core, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return parsed{}, false
	}
	for i, s := range parts {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			return parsed{}, false
		}
		p.core[i] = n
	}
	return p, true
}

func comparePre(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		na, errA := strconv.Atoi(a[i])
		nb, errB := strconv.Atoi(b[i])
		var c int
		switch {
		case errA == nil && errB == nil:
			c = compareInt(na, nb)
		case errA == nil:
			c = -1
		case errB == nil:
			c = 1
		default:
			c = strings.Compare(a[i], b[i])
		}
		if c != 0 {
			return c
		}
	}
	return compareInt(len(a), len(b))
}

func compareInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
