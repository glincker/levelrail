package api

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const hardeningDocsPath = "/security#container-hardening"

// doctorCheckContainerHardening reports this control plane's own policy;
// remote agents read their own APP_CONTAINER_HARDENING.
func doctorCheckContainerHardening(cfg docker.HardeningConfig, cfgErr error) doctorCheckResource {
	const code, name = "container_hardening", "Container hardening"
	r := cfg.Report()
	detail := fmt.Sprintf("cap_drop=ALL, cap_add=%s, no-new-privileges, pids_limit=%d", strings.Join(r.CapAdd, ","), r.PidsLimit)
	fix := "Set APP_CONTAINER_HARDENING=enforce to apply these defaults to new containers; extra capabilities go in APP_CONTAINER_HARDENING_CAP_ADD."
	warn := func(msg string) doctorCheckResource {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: msg, Fix: fix, DocsPath: hardeningDocsPath}
	}
	switch {
	case cfgErr != nil:
		return warn(cfgErr.Error() + "; using safe defaults")
	case r.Applied:
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "enforce: " + detail}
	case r.Mode == docker.HardeningOff:
		return warn("off: containers run with Docker's default capabilities")
	default:
		return warn("warn (not applied): enforce would set " + detail)
	}
}
