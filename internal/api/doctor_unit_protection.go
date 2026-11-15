package api

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/runmarker"
)

const (
	procOOMScoreAdj  = "/proc/self/oom_score_adj"
	procMeminfoPath  = "/proc/meminfo"
	protectionGlob   = "/etc/systemd/system/*.service.d/30-protection.conf"
	protectionMarker = "host protection"
	envProtectedAdj  = "APP_PROTECTED_OOM_SCORE_ADJ"
	defaultProtected = -500
)

// doctorCheckUnitProtection reports whether the running process is shielded
// from the kernel OOM killer and whether it was recently killed. Swap is shown
// for context only; the host pressure checks own the swap verdict.
func (rt *Router) doctorCheckUnitProtection(now time.Time) doctorCheckResource {
	const code, name = "unit_protection", "Control plane protection"
	raw, err := os.ReadFile(procOOMScoreAdj)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "The OOM score is not readable on this operating system, so protection was not checked."}
	}
	adj, perr := strconv.Atoi(strings.TrimSpace(string(raw)))
	if perr != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "The OOM score could not be parsed."}
	}
	limit := defaultProtected
	if v, e := strconv.Atoi(os.Getenv(envProtectedAdj)); e == nil {
		limit = v
	}
	protected := adj <= limit
	dropin := protectionDropinExists()
	facts := fmt.Sprintf("OOM score adjustment %d, drop-in %s, %s", adj, presence(dropin), swapSummary())

	var recent []runmarker.Incident
	if rt.dataDir != "" {
		recent = runmarker.Recent(rt.dataDir, runmarker.WindowFromEnv(os.Getenv), now)
	}
	switch {
	case len(recent) > 0:
		oom := false
		for _, in := range recent {
			oom = oom || in.OOMLikely
		}
		msg := fmt.Sprintf("%s; %d unclean restart(s) recently", facts, len(recent))
		if oom {
			msg += ", likely out of memory"
		}
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: msg,
			Fix: "Add swap, lower build concurrency, set app memory limits and check which containers use the most memory.", DocsPath: "/host-protection"}
	case !protected && dropin:
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: facts + "; the drop-in is written but not applied yet",
			Fix: "Restart the service to apply the drop-in: systemctl restart <service>.", DocsPath: "/host-protection"}
	case !protected:
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: facts + "; the kernel may pick the control plane first when memory runs out",
			Fix: "Run the installer's protect command as root, then restart the service.", DocsPath: "/host-protection"}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: facts}
}

func protectionDropinExists() bool {
	files, _ := filepath.Glob(protectionGlob)
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil && bytes.Contains(bytes.ToLower(b), []byte(protectionMarker)) { //nolint:gosec // glob under /etc/systemd/system
			return true
		}
	}
	return false
}

func presence(ok bool) string {
	if ok {
		return "present"
	}
	return "missing"
}

func swapSummary() string {
	b, err := os.ReadFile(procMeminfoPath)
	if err != nil {
		return "swap unknown"
	}
	for _, line := range strings.Split(string(b), "\n") {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "SwapTotal:" {
			if kb, e := strconv.ParseInt(f[1], 10, 64); e == nil {
				if kb == 0 {
					return "no swap"
				}
				return "swap " + humanBytes(kb<<10)
			}
		}
	}
	return "swap unknown"
}
