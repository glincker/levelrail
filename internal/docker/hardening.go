package docker

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
)

// HardeningMode selects whether secure container defaults are applied.
type HardeningMode string

const (
	// HardeningOff applies nothing and reports nothing.
	HardeningOff HardeningMode = "off"
	// HardeningWarn applies nothing to containers; diagnostics report what
	// enforce would set. Default, so existing deployments cannot break.
	HardeningWarn HardeningMode = "warn"
	// HardeningEnforce drops all capabilities except the minimal set, sets
	// no-new-privileges and a PID limit on every container Create makes.
	HardeningEnforce HardeningMode = "enforce"
)

const (
	envHardening     = "APP_CONTAINER_HARDENING"
	envHardeningCaps = "APP_CONTAINER_HARDENING_CAP_ADD"
	envHardeningPids = "APP_CONTAINER_PIDS_LIMIT"

	// DefaultPidsLimit is generous enough for forking servers and database
	// backends while still stopping a fork bomb from exhausting the host.
	DefaultPidsLimit int64 = 4096

	securityOptNoNewPrivileges = "no-new-privileges"
)

// MinimalCapabilities is what ordinary images need to start after a
// drop-all: entrypoints that chown data dirs and drop privileges (postgres,
// mysql, redis via gosu/su-exec: CHOWN, FOWNER, FSETID, DAC_OVERRIDE,
// SETUID, SETGID), nginx/node/python binding ports below 1024
// (NET_BIND_SERVICE), and init scripts signalling other users' processes
// (KILL). Docker's default set also grants NET_RAW, MKNOD, SETFCAP,
// SETPCAP, SYS_CHROOT and AUDIT_WRITE, deliberately left out.
var MinimalCapabilities = []string{
	"CHOWN", "DAC_OVERRIDE", "FOWNER", "FSETID", "KILL",
	"NET_BIND_SERVICE", "SETGID", "SETUID",
}

// HardeningConfig is the effective container hardening policy.
type HardeningConfig struct {
	Mode HardeningMode
	// PidsLimit is the per-container process cap; 0 or negative disables it.
	PidsLimit int64
	// ExtraCaps are operator-granted capabilities added on top of
	// MinimalCapabilities for every container.
	ExtraCaps []string
}

// HardeningReport describes what enforce sets, for the doctor endpoint.
type HardeningReport struct {
	Mode            HardeningMode
	Applied         bool
	CapDrop         []string
	CapAdd          []string
	NoNewPrivileges bool
	PidsLimit       int64
}

// ParseHardeningMode validates a mode string; empty means the default.
func ParseHardeningMode(s string) (HardeningMode, error) {
	switch m := HardeningMode(strings.ToLower(strings.TrimSpace(s))); m {
	case "":
		return HardeningWarn, nil
	case HardeningOff, HardeningWarn, HardeningEnforce:
		return m, nil
	default:
		return HardeningWarn, fmt.Errorf("docker: %s=%q is not one of off, warn, enforce", envHardening, s)
	}
}

// HardeningFromEnv reads the hardening policy from the environment. On an
// invalid value it returns the safe default (warn) together with the error.
func HardeningFromEnv() (HardeningConfig, error) {
	cfg := HardeningConfig{PidsLimit: DefaultPidsLimit}
	mode, firstErr := ParseHardeningMode(os.Getenv(envHardening))
	cfg.Mode = mode
	if v := strings.TrimSpace(os.Getenv(envHardeningPids)); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		switch {
		case err != nil || n < 0:
			if firstErr == nil {
				firstErr = fmt.Errorf("docker: %s=%q is not a non-negative integer", envHardeningPids, v)
			}
		default:
			cfg.PidsLimit = n
		}
	}
	for _, c := range strings.Split(os.Getenv(envHardeningCaps), ",") {
		if c = strings.ToUpper(strings.TrimSpace(c)); c != "" {
			cfg.ExtraCaps = append(cfg.ExtraCaps, strings.TrimPrefix(c, "CAP_"))
		}
	}
	return cfg, firstErr
}

// WithHardening overrides the policy NewClient reads from the environment.
func WithHardening(cfg HardeningConfig) ClientOption {
	return func(c *Client) { c.hardening = cfg }
}

func (h HardeningConfig) enforced() bool { return h.Mode == HardeningEnforce }

func (h HardeningConfig) capAdd(extra []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, group := range [][]string{MinimalCapabilities, h.ExtraCaps, extra} {
		for _, c := range group {
			c = strings.TrimPrefix(strings.ToUpper(c), "CAP_")
			if c != "" && !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Report returns what enforce sets, and whether it is actually applied.
func (h HardeningConfig) Report() HardeningReport {
	if h.Mode == HardeningOff {
		return HardeningReport{Mode: h.Mode}
	}
	return HardeningReport{
		Mode:            h.Mode,
		Applied:         h.enforced(),
		CapDrop:         []string{"ALL"},
		CapAdd:          h.capAdd(nil),
		NoNewPrivileges: true,
		PidsLimit:       h.PidsLimit,
	}
}

// apply hardens hc in enforce mode; spec.CapAdd survives the drop-all.
func (h HardeningConfig) apply(hc *container.HostConfig, spec ContainerSpec) {
	if !h.enforced() {
		return
	}
	hc.CapDrop = []string{"ALL"}
	hc.CapAdd = h.capAdd(spec.CapAdd)
	hc.SecurityOpt = append(hc.SecurityOpt, securityOptNoNewPrivileges)
	if h.PidsLimit > 0 && hc.PidsLimit == nil {
		limit := h.PidsLimit
		hc.PidsLimit = &limit
	}
}

func logHardeningEnv(err error) {
	if err != nil {
		slog.Warn("container hardening config invalid, using defaults", slog.String("error", err.Error()))
	}
}
