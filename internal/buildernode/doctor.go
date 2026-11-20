package buildernode

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Check is one doctor result.
type Check struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail"`
	// Fix is one actionable line, set only on a failing check.
	Fix string `json:"fix,omitempty"`
}

// fixes maps each check to the single step that most often repairs it.
var fixes = map[string]string{
	"arch":          "Use an arm64 or amd64 machine as a builder.",
	"ram":           "Free memory or lower the VM size with `levelrail builder start --max-mem MiB`.",
	"disk":          "Free disk space, build images and caches live under the VM directory.",
	"lima":          "Run `brew install lima`, then `levelrail builder doctor` again.",
	"vm":            "Run `limactl list` to inspect the builder VM, then `levelrail builder start`.",
	"control-plane": "Run `levelrail builders add` on the control plane, then `levelrail builder join` with the printed line.",
	"clock":         "Turn on automatic date and time in system settings so certificates validate.",
}

// DoctorEnv is everything doctor reads from the host, injected so tests run offline.
type DoctorEnv struct {
	Arch         string
	HostMemMiB   int
	DiskFreeGiB  int
	LimaPath     func() (string, error)
	Runner       Runner
	ServerTime   func(ctx context.Context, server string) (time.Time, error)
	Now          func() time.Time
	Env          Lookup
	ConfigServer string
}

// VMState returns the Lima status of the builder instance: "Running", "Stopped" or "" when absent.
func VMState(ctx context.Context, r Runner) (string, error) {
	out, err := r.Run(ctx, "list", "--format", "{{.Name}} {{.Status}}")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && f[0] == Instance {
			return f[1], nil
		}
	}
	return "", nil
}

// Doctor runs every check and never stops at the first failure.
func Doctor(ctx context.Context, e DoctorEnv) ([]Check, error) {
	minRAM, err := intEnv(e.Env, EnvMinRAMMiB, DefaultMinRAMMiB)
	if err != nil {
		return nil, err
	}
	minDisk, err := intEnv(e.Env, EnvMinDiskGiB, DefaultMinDisk)
	if err != nil {
		return nil, err
	}
	skew, err := durEnv(e.Env, EnvMaxClockSkew, DefaultSkew)
	if err != nil {
		return nil, err
	}
	var out []Check
	add := func(name string, ok bool, detail string) {
		c := Check{Name: name, OK: ok, Detail: detail}
		if !ok {
			c.Fix = fixes[name]
		}
		out = append(out, c)
	}

	switch e.Arch {
	case "arm64", "aarch64", "amd64", "x86_64":
		add("arch", true, e.Arch)
	default:
		add("arch", false, fmt.Sprintf("%s is not supported; the builder needs arm64 or amd64", e.Arch))
	}
	add("ram", e.HostMemMiB >= minRAM, fmt.Sprintf("%d MiB installed, %d MiB wanted (%s)", e.HostMemMiB, minRAM, EnvMinRAMMiB))
	add("disk", e.DiskFreeGiB >= minDisk, fmt.Sprintf("%d GiB free, %d GiB wanted (%s)", e.DiskFreeGiB, minDisk, EnvMinDiskGiB))

	if _, lerr := e.LimaPath(); lerr != nil {
		add("lima", false, "limactl not found; install it with `brew install lima`")
		add("vm", false, "unknown without limactl")
	} else {
		add("lima", true, "limactl found")
		st, serr := VMState(ctx, e.Runner)
		switch {
		case serr != nil:
			add("vm", false, serr.Error())
		case st == "":
			add("vm", true, "not created yet; `levelrail builder start` creates it")
		default:
			add("vm", true, st)
		}
	}

	if e.ConfigServer == "" {
		add("control-plane", false, "not joined; run `levelrail builder join`")
		add("clock", false, "needs a control plane to compare against")
		return out, nil
	}
	st, terr := e.ServerTime(ctx, e.ConfigServer)
	if terr != nil {
		add("control-plane", false, terr.Error())
		add("clock", false, "needs a reachable control plane")
		return out, nil
	}
	add("control-plane", true, "reachable over TLS")
	d := e.Now().Sub(st)
	if d < 0 {
		d = -d
	}
	add("clock", d <= skew, fmt.Sprintf("skew %s, limit %s (%s)", d.Round(time.Second), skew, EnvMaxClockSkew))
	return out, nil
}

// Passed reports whether every check is OK.
func Passed(cs []Check) bool {
	for _, c := range cs {
		if !c.OK {
			return false
		}
	}
	return true
}
