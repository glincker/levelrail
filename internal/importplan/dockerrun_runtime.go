package importplan

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/spec"
)

// applyRuntimeFlag records a supported tuning flag and reports whether it was one.
func (dr *dockerRun) applyRuntimeFlag(flag, val string) bool {
	r := &dr.Runtime
	switch flag {
	case "--shm-size":
		r.ShmSize = strings.ToLower(val)
	case "--tmpfs":
		r.Tmpfs = append(r.Tmpfs, val)
	case "--ulimit":
		name, rest, _ := strings.Cut(val, "=")
		soft, hard, hasHard := strings.Cut(rest, ":")
		s, errS := strconv.ParseInt(soft, 10, 64)
		h := s
		var errH error
		if hasHard {
			h, errH = strconv.ParseInt(hard, 10, 64)
		}
		if errS != nil || errH != nil {
			dr.Warnings = append(dr.Warnings, Warning{Code: "unsupported_flag", Message: "--ulimit: expected name=soft[:hard] with integers (not supported, ignored)"})
			return true
		}
		if r.Ulimits == nil {
			r.Ulimits = map[string]spec.Ulimit{}
		}
		r.Ulimits[name] = spec.Ulimit{Soft: s, Hard: h}
	case "--init":
		on := true
		r.Init = &on
	case "--read-only":
		on := true
		r.ReadOnly = &on
	case "--stop-timeout":
		n, err := strconv.Atoi(val)
		if err != nil {
			dr.Warnings = append(dr.Warnings, Warning{Code: "unsupported_flag", Message: "--stop-timeout: expected seconds (not supported, ignored)"})
			return true
		}
		r.StopTimeout = &n
	case "--add-host":
		r.ExtraHosts = append(r.ExtraHosts, val)
	case "--dns":
		r.DNS = append(r.DNS, val)
	case "--sysctl":
		k, v, _ := strings.Cut(val, "=")
		if r.Sysctls == nil {
			r.Sysctls = map[string]string{}
		}
		r.Sysctls[k] = v
	case "--cap-drop":
		r.CapDrop = append(r.CapDrop, strings.TrimPrefix(strings.ToUpper(val), "CAP_"))
	case "--pids-limit":
		n, err := strconv.Atoi(val)
		if err != nil || n < 1 {
			dr.Warnings = append(dr.Warnings, Warning{Code: "unsupported_flag", Message: "--pids-limit: expected a positive number (not supported, ignored)"})
			return true
		}
		r.PidsLimit = n
	default:
		return false
	}
	return true
}

// finishRuntime validates the collected options; a rejected set is dropped whole with the reason.
func (dr *dockerRun) finishRuntime() {
	if err := dr.Runtime.Validate(); err != nil {
		dr.Warnings = append(dr.Warnings, Warning{Code: "unsupported_flag", Message: fmt.Sprintf("runtime tuning flags were not imported: %v", err)})
		dr.Runtime = spec.RuntimeOptions{}
	}
}

// runtimeKeys renders the options as compose keys, nil when none were given.
func (dr *dockerRun) runtimeKeys() map[string]any {
	keys := dr.Runtime.ComposeKeys()
	if len(keys) == 0 {
		return nil
	}
	return keys
}
