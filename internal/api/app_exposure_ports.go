package api

import (
	"fmt"
	"hash/fnv"
	"os"
	"strconv"
	"strings"
)

// envExposurePortRange overrides the host port range auto-picked for direct
// exposure ("lo-hi"). The default stays below Docker's ephemeral range so an
// auto-picked port never fights a Docker-assigned one.
const envExposurePortRange = "APP_EXPOSURE_PORT_RANGE"

const (
	defaultExposurePortLo = 20000
	defaultExposurePortHi = 29999
)

// Kinds of holder a host port can have, shown verbatim in refusal messages.
const (
	holderKindControl  = "control plane"
	holderKindApp      = "app"
	holderKindStream   = "stream"
	holderKindListener = "listener"
)

// portHolder says who owns a host port on this node.
type portHolder struct {
	Port   int    `json:"port"`
	Kind   string `json:"kind"`
	Holder string `json:"holder"`
}

func (h portHolder) describe() string {
	if h.Kind == holderKindListener {
		return "another process on this server"
	}
	return h.Kind + " " + h.Holder
}

// exposurePortRange reads APP_EXPOSURE_PORT_RANGE, falling back to the
// default on a missing or malformed value.
func exposurePortRange(getenv func(string) string) (lo, hi int) {
	raw := strings.TrimSpace(getenv(envExposurePortRange))
	if raw == "" {
		return defaultExposurePortLo, defaultExposurePortHi
	}
	a, b, ok := strings.Cut(raw, "-")
	l, errL := strconv.Atoi(strings.TrimSpace(a))
	h, errH := strconv.Atoi(strings.TrimSpace(b))
	if !ok || errL != nil || errH != nil || l < 1024 || h > 65535 || l > h {
		return defaultExposurePortLo, defaultExposurePortHi
	}
	return l, h
}

func osGetenv(k string) string { return os.Getenv(k) }

func validateExposurePort(field string, p int) error {
	if p < 1 || p > 65535 {
		return fmt.Errorf("%s must be between 1 and 65535", field)
	}
	return nil
}

// pickHostPort returns the first free port in [lo, hi], scanning from a
// start derived from the app name so an app keeps a stable suggestion.
func pickHostPort(app string, lo, hi int, taken map[int]portHolder) (int, bool) {
	span := hi - lo + 1
	if span <= 0 {
		return 0, false
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(app))
	start := int(h.Sum32() % uint32(span)) //nolint:gosec // span is bounded by 65535
	for i := 0; i < span; i++ {
		p := lo + (start+i)%span
		if _, busy := taken[p]; !busy {
			return p, true
		}
	}
	return 0, false
}
