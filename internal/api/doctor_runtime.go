package api

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

const containerRuntimeDocsPath = "/security#rootless-and-podman"

// runtimeReporter is the Docker client behind DockerPinger, narrowed the
// same consumer-defined way engineVersioner is above (updates_preflight.go):
// *docker.Client satisfies it structurally via Runtime.
type runtimeReporter interface {
	Runtime() docker.RuntimeInfo
}

// doctorCheckContainerRuntime reports which container engine and
// privilege mode this control plane is likely talking to. It prefers the
// live client's own resolved RuntimeInfo (rt.dockerPinger, reflecting any
// APP_CONTAINER_RUNTIME_SOCKET override actually in effect) and falls
// back to a fresh environment-only detection when no Docker client is
// configured, so the check still reports something before Docker is
// reachable. Rootless and Podman are flagged warn, not fail: this
// platform's own support for them is best-effort (see docs/security.md),
// not a broken state.
func (rt *Router) doctorCheckContainerRuntime(ctx context.Context) doctorCheckResource {
	const code, name = "container_runtime", "Container runtime"

	info := docker.DetectRuntimeSocket(os.LookupEnv)
	if reporter, ok := rt.dockerPinger.(runtimeReporter); ok {
		info = reporter.Runtime()
	}
	if v, ok := rt.dockerPinger.(engineVersioner); ok {
		if version, err := v.ServerVersion(ctx); err == nil && strings.Contains(strings.ToLower(version), "podman") {
			info.Kind = docker.RuntimeKindPodman
		}
	}

	mode := "rootful"
	if info.Rootless {
		mode = "rootless"
	}
	detail := fmt.Sprintf("%s (%s), socket resolved from %s: %s", info.Kind, mode, info.Source, info.Host)

	if info.Kind != docker.RuntimeKindPodman && !info.Rootless {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: detail}
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message:  detail + "; rootless/Podman support is best-effort, verify bind-mount ownership and UID mapping before relying on it",
		DocsPath: containerRuntimeDocsPath,
	}
}
