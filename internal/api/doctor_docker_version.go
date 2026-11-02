package api

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// defaultMinDockerMajor matches the server readiness check; the same env
// var overrides both so the two never disagree.
const defaultMinDockerMajor = 24

// doctorCheckDockerVersion warns when the engine is older than the
// oldest major we test against.
func (rt *Router) doctorCheckDockerVersion(ctx context.Context) doctorCheckResource {
	const code, name = "docker_version", "Docker engine version"
	v, ok := rt.dockerPinger.(engineVersioner)
	if !ok {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "engine version is not available"}
	}
	if rt.detectRuntimeInfo(ctx).Kind == docker.RuntimeKindPodman {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "Podman uses its own version line, not compared"}
	}
	ver, err := v.ServerVersion(ctx)
	if err != nil || ver == "" {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "engine version could not be read"}
	}
	minMajor := int(envFloat("APP_READINESS_MIN_DOCKER_MAJOR", defaultMinDockerMajor))
	major, err := strconv.Atoi(strings.SplitN(strings.TrimPrefix(ver, "v"), ".", 2)[0])
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "engine version " + ver + " is not a recognised format"}
	}
	if major < minMajor {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn,
			Message: fmt.Sprintf("engine %s is older than the minimum tested version %d", ver, minMajor),
			Fix:     "Upgrade Docker Engine to " + strconv.Itoa(minMajor) + " or newer."}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("engine %s (minimum %d)", ver, minMajor)}
}
