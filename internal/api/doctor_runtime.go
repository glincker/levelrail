package api

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/GLINCKER/levelrail/internal/docker"
)

// runtimeGOOS is a seam so tests can grade support for any platform.
var runtimeGOOS = runtime.GOOS

const containerRuntimeDocsPath = "/container-runtimes"

// runtimeReporter is the Docker client behind DockerPinger, narrowed the
// same consumer-defined way engineVersioner is above (updates_preflight.go):
// *docker.Client satisfies it structurally via Runtime.
type runtimeReporter interface {
	Runtime() docker.RuntimeInfo
}

// runtimeIdentifier is the Docker client's /version and /info lookup.
type runtimeIdentifier interface {
	EngineIdentity(ctx context.Context) (docker.EngineIdentity, error)
}

// doctorCheckContainerRuntime reports which container engine, privilege
// mode and support level this control plane has. It prefers the live
// client's resolved RuntimeInfo (reflecting any APP_CONTAINER_RUNTIME_SOCKET
// override) and falls back to environment-only detection, then refines the
// kind from the engine's own answer. Anything short of rootful Docker on
// Linux is warn, never fail: it is best-effort, not broken (docs/security.md).
func (rt *Router) doctorCheckContainerRuntime(ctx context.Context) doctorCheckResource {
	const code, name = "container_runtime", "Container runtime"

	info := rt.detectRuntimeInfo(ctx)

	mode := "rootful"
	if info.Rootless {
		mode = "rootless"
	}
	level, reason := docker.SupportFor(info, runtimeGOOS)
	detail := fmt.Sprintf("%s (%s), socket resolved from %s: %s", info.Kind, mode, info.Source, info.Host)
	res := doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusOK,
		EngineKind: string(info.Kind), SupportLevel: string(level),
		Message: fmt.Sprintf("%s; support level %s (%s)", detail, level, reason),
	}
	if level == docker.SupportSupported {
		return res
	}
	res.Status = doctorStatusWarn
	res.DocsPath = containerRuntimeDocsPath
	if info.Kind == docker.RuntimeKindPodman || info.Kind == docker.RuntimeKindUnknown {
		res.Message += "; image and compose deploys are expected to work, source builds (Dockerfile, Railpack) need BuildKit inside dockerd and are not verified here"
	}
	res.Message += "; if this is the wrong engine, set APP_CONTAINER_RUNTIME_SOCKET to its socket (for Podman, enable podman.socket first). See the container runtimes guide"
	return res
}

// detectRuntimeInfo resolves the engine the way the container_runtime check does.
func (rt *Router) detectRuntimeInfo(ctx context.Context) docker.RuntimeInfo {
	info := docker.DetectRuntimeSocket(os.LookupEnv)
	if reporter, ok := rt.dockerPinger.(runtimeReporter); ok {
		info = reporter.Runtime()
	}
	if idr, ok := rt.dockerPinger.(runtimeIdentifier); ok {
		if id, err := idr.EngineIdentity(ctx); err == nil {
			info.Kind = docker.RefineKind(info.Kind, id)
		}
	} else if v, ok := rt.dockerPinger.(engineVersioner); ok {
		if version, err := v.ServerVersion(ctx); err == nil && strings.Contains(strings.ToLower(version), "podman") {
			info.Kind = docker.RuntimeKindPodman
		}
	}
	return info
}

// relabelEngine swaps the leading "Docker" of a row name for the detected
// engine, so Podman hosts do not read "Docker daemon".
func (rt *Router) relabelEngine(f func(ctx context.Context) doctorCheckResource) func(ctx context.Context) doctorCheckResource {
	return func(ctx context.Context) doctorCheckResource {
		res := f(ctx)
		if rest, ok := strings.CutPrefix(res.Name, "Docker "); ok && rt.detectRuntimeInfo(ctx).Kind == docker.RuntimeKindPodman {
			res.Name = "Podman " + rest
		}
		return res
	}
}
